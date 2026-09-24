package boolindex

import (
	"slices"
	"sync"
)

// scratch 是一次查询用到的全部临时缓冲。它挂在 sync.Pool 上复用，
// 让热路径只留下「结果切片」这一次必然的分配。
//
// 用法上有个约束：gather 期间不许再分配 cursors。归并会让每条游标自带位置状态，
// 如果 append 触发了扩容，先建好的游标组会指向旧数组，新旧两份状态就此分叉。
// 所以 gather 先统计画像取值总数再一次性备足容量。
type scratch struct {
	groups  []fieldCursor
	cursors []cursor
	dup     []string
	ids     []uint64

	// uncond 是给「无正向条件的定向包」预留的游标槽位。
	// 这个游标组每条查询都要重建（游标带位置状态），用固定槽位避免每次都分配。
	uncond [1]cursor
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

// maxPooledBuffer 是回池的最大缓冲长度。单次查询结果特别大时不再复用它，
// 免得池里长期占着一块大数组。
const maxPooledBuffer = 1 << 16

func acquireScratch() *scratch { return scratchPool.Get().(*scratch) }

func releaseScratch(sc *scratch) {
	sc.groups = sc.groups[:0]
	sc.cursors = sc.cursors[:0]
	sc.dup = sc.dup[:0]
	if cap(sc.ids) > maxPooledBuffer {
		sc.ids = nil
	} else {
		sc.ids = sc.ids[:0]
	}
	if cap(sc.cursors) > maxPooledBuffer {
		sc.cursors = nil
	}
	scratchPool.Put(sc)
}

// gather 把一次画像转换成一层的字段游标组。
//
// 四个要点：
//   - 每个字段只产生一个游标组，组内是画像该字段各取值的游标
//   - 画像取值先按字段去重：同一个取值出现两次会造出两条指向同一倒排链的游标，
//     让同一个字段被当成两个字段，凑出假阳性
//   - 数值字段的取值在这里解析成整数，解析不了的取值什么都命中不了
//   - 只有排除条件（size=0）的定向包不挂在任何字段值上，共用一个「无条件」游标组，
//     它的正向记录在归并里靠 need=max(1,size)=1 生效
func (ix *Indexer) gather(p Profile, sc *scratch) []fieldCursor {
	total := 0
	for _, values := range p {
		total += len(values)
	}
	sc.ensureCursors(total)

	sc.groups = sc.groups[:0]
	for f, values := range p {
		h := ix.fields[f]
		if h == nil {
			continue // 画像里有、索引里没有的字段：没有任何定向包约束它，直接忽略
		}
		start := len(sc.cursors)
		sc.dup = sc.dup[:0]
		for _, v := range values {
			if slices.Contains(sc.dup, v) {
				continue
			}
			sc.dup = append(sc.dup, v)
			if list := h.list(v); len(list) > 0 {
				sc.cursors = append(sc.cursors, newCursor(list))
			}
		}
		if len(sc.cursors) > start {
			sc.groups = append(sc.groups, newFieldCursor(sc.cursors[start:]))
		}
	}

	if len(ix.unconditional) > 0 {
		sc.uncond[0] = newCursor(ix.unconditional)
		sc.groups = append(sc.groups, newFieldCursor(sc.uncond[:]))
	}
	return sc.groups
}

// ensureCursors 备足游标缓冲，保证 gather 期间 append 不会再扩容
func (sc *scratch) ensureCursors(n int) {
	if cap(sc.cursors) < n {
		sc.cursors = make([]cursor, 0, n)
	}
	sc.cursors = sc.cursors[:0]
}

// match 是归并检索核心：把各字段游标组按当前记录升序归并，逐一定向包判定。
//
// 判定一个定向包 C 需要 need = max(1, C.size()) 个字段游标组的 head 同时落在 C 上，
// 也就是 C 要求的每个正向字段都被画像满足。因为 size 在 entryID 高位，
// 归并过程中 head 的 size 单调不减，所以一旦 need 超过剩余游标组数就可以整体结束。
func (ix *Indexer) match(p Profile, sc *scratch, dst []uint64) []uint64 {
	groups := ix.gather(p, sc)
	sortFieldCursors(groups)

	for len(groups) > 0 {
		eid := groups[0].headEID
		conj := eid.conjID()

		// size=0 的定向包（只有排除条件）也需要 1 个游标就能判定，
		// 这里的 1 不是随手写的：改成 max(0,size) 会让这类包的排除语义失效——
		// 排除记录永远排在正向记录之前，靠 need=1 才能让 head 落在排除记录上走排除分支。
		need := max(1, conj.size())
		// 当前最小的 size 都凑不齐游标，更大的 size 更不可能，可以结束
		if need > len(groups) {
			break
		}

		endEID := groups[need-1].headEID
		next := endEID.conjID() // 默认直接跳过 endEID 所在的定向包

		if endEID.conjID() == conj {
			// need 个字段的 head 都落在同一个定向包上 -> 该定向包的正向条件全部满足
			next = conj.nextConj()
			if eid.isInclude() {
				dst = append(dst, conj.ruleID())
			} else {
				// 命中的是排除记录，该定向包判负。
				// 还要把后面仍停留在本定向包上的游标一起推走：本定向包的记录只有
				// conjID 与 conjID+1 两个取值，它们比 next 小，不清掉下一轮会被
				// 当成「正向齐备」重新拾起，导致排除失效。
				for i := need; i < len(groups); i++ {
					if groups[i].headEID < next {
						groups[i].skipTo(next)
					}
				}
			}
		}

		for i := 0; i < need; i++ {
			groups[i].skipTo(next)
		}

		sortFieldCursors(groups)
		// 已耗尽的游标组排序后必然在尾部，直接截掉
		for len(groups) > 0 && groups[len(groups)-1].headEID == nullEntry {
			groups = groups[:len(groups)-1]
		}
	}
	return dst
}
