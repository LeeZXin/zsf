package boolindex

// entries 是一条倒排链：某个 (字段, 取值) 命中的全部定向包记录，构建期排好序，查询期只读。
type entries []entryID

// cursor 是一条倒排链上的游标。
//
// cur 缓存 list[pos] 的值，耗尽时为 nullEntry。归并每轮要对游标做大量比较与推进，
// 每次现算 cur 都要多一次长度判断和边界检查（实测占掉一半 CPU），所以缓存下来。
// 缓存的代价是每个改动 pos 的地方都必须同步更新 cur，新增出口时留意这一点。
// TestCursorCacheInvariant 断言了两者始终一致。
type cursor struct {
	list entries
	pos  int
	cur  entryID
}

func newCursor(list entries) cursor {
	c := cursor{list: list}
	c.cur = c.at(c.pos)
	return c
}

// at 读 list[i]，越界返回 nullEntry
func (c *cursor) at(i int) entryID {
	if i >= len(c.list) {
		return nullEntry
	}
	return c.list[i]
}

// skipTo 把游标推进到第一条 >= id 的记录并返回它，越界返回 nullEntry。
//
// 契约是「第一个 >= id」，不能写成「第一个 > id」：归并里 next = 定向包标识 + 2
// 依赖这个语义精确落在下一条记录（相邻定向包）的排除位上，写成 > 会漏掉它。
//
// 实现用倍增试探 + 二分：归并时目标位置通常离当前位置很近，倍增能跳过线性扫描；
// 目标很远时退化成 log 次倍增，再用二分收敛到区间内。
func (c *cursor) skipTo(id entryID) entryID {
	if c.list == nil {
		return nullEntry
	}
	if c.cur >= id {
		return c.cur
	}

	// 倍增：右界从 pos+1 开始翻倍，直到落在 id 之后或触底。
	// 顺手把左界收到最后一个已知 < id 的位置，二分区间能窄一半。
	base := c.pos
	left := base
	right := base + 1
	for right < len(c.list) && c.list[right] < id {
		left = right
		right = base + (right-base)*2
	}
	if right > len(c.list) {
		right = len(c.list)
	}

	// 二分收敛：答案在 [left, right) 内，且 list[left] < id 已知
	for left < right {
		mid := left + (right-left)/2
		if c.list[mid] >= id {
			right = mid
		} else {
			left = mid + 1
		}
	}

	c.pos = left
	c.cur = c.at(left)
	return c.cur
}

// fieldCursor 是「一个字段」上的游标组：组内每个画像取值一条游标，head 取组内最小记录。
//
// 必须按字段分组，不能把 (字段, 取值) 铺平成一层游标。定向包的 size 是
// 「有正向条件的字段个数」，归并用 need <= 游标组数 来判断「这个包凑不齐正向字段」；
// 铺平之后同一个字段的多个取值会被当成多个字段，产生假阳性：
//
//	包 {tag: In[a,x], city: In[bj]}（size=2），画像只给 tag=[a,x]
//	铺平时会凑出 2 条游标且都指向该包 -> 误判命中，而 city 根本没出现在画像里。
//
// headIdx 存下标而不是指针：组内游标会随推进改变位置，存指针容易留下失效引用。
type fieldCursor struct {
	cursors []cursor
	headIdx int
	headEID entryID
}

func newFieldCursor(cs []cursor) fieldCursor {
	fc := fieldCursor{cursors: cs}
	best := 0
	for i := range fc.cursors {
		if fc.cursors[i].cur < fc.cursors[best].cur {
			best = i
		}
	}
	fc.headIdx = best
	fc.headEID = fc.cursors[best].cur
	return fc
}

// skipTo 推进组内全部游标到 >= id，返回新的 head。
// 组内所有游标都要推进：该字段的任一取值可能命中的记录都必须越过 id。
func (fc *fieldCursor) skipTo(id entryID) entryID {
	best := 0
	for i := range fc.cursors {
		fc.cursors[i].skipTo(id)
		if fc.cursors[i].cur < fc.cursors[best].cur {
			best = i
		}
	}
	fc.headIdx = best
	fc.headEID = fc.cursors[best].cur
	return fc.headEID
}

// sortFieldCursors 按 head 升序原地排序。
// 游标组数量很少（等于画像字段数），插入排序比 sort.Sort 的接口开销更划算。
func sortFieldCursors(fcs []fieldCursor) {
	for i := 1; i < len(fcs); i++ {
		for j := i; j > 0 && fcs[j].headEID < fcs[j-1].headEID; j-- {
			fcs[j], fcs[j-1] = fcs[j-1], fcs[j]
		}
	}
}
