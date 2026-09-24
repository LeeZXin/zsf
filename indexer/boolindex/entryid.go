package boolindex

// entryID 把「属于哪条规则的哪个定向包」「该定向包有几个正向条件」「这条倒排记录是正向还是排除」
// 打包进一个 uint64。这样归并时只需比较 entryID，不必额外查表定位定向包，
// 而且排序本身就带着算法需要的全部信息：
//
//	bit 63..56  size     定向包的正向字段个数，取值 0..255
//	bit 55..12  ruleID   规则 ID（广告 ID），取值 0..2^44-1
//	bit 11..4   conjIdx  定向包在规则内的序号，取值 0..255
//	bit 3..1    保留，恒为 0
//	bit 0       incl     1 正向记录 / 0 排除记录
//
// 三条性质是整个检索算法的基础，改动位布局前必须保证它们仍然成立：
//
//  1. 同一定向包的排除记录必然排在正向记录之前（只有 bit0 不同），排除因此天然优先生效。
//  2. 正向记录 + 1 即跳过该定向包的全部记录：conjIdx 占 bit4..11，相邻定向包差 16，
//     而 +2 仍落在低 4 位里，不会进位到相邻定向包。
//  3. size 在高位，所以每条 posting list 天然按 (size, ruleID, conjIdx) 升序，
//     归并过程中 size 单调不减，可以安全提前退出。
//
// 保留位恒为 0 是性质 2 里 +1 不回绕的前提：最大合法 entryID 是 0xFFFFFFFFFFFFFFF1，
// 严格小于 nullEntry。TestEntryIDLayout 断言了这一点。
type entryID uint64

const (
	// maxSize 与 maxConjIdx 都只有 8 bit，超出的定向包必须报错而不是截断
	maxSize    = 255
	maxConjIdx = 255

	// maxRuleID 44 bit，约 1.7e13，广告 ID 用不完
	maxRuleID = 1<<44 - 1

	// nullEntry 是游标耗尽的哨兵。它必须大于任何合法 entryID，
	// 归并末尾依赖「游标到达 nullEntry」判断该组可以丢弃。
	nullEntry entryID = ^entryID(0)

	sizeShift    = 56
	ruleIDShift  = 12
	conjIdxShift = 4

	inclBit = entryID(1)

	// conjMask 抹掉 incl 与保留位，得到「定向包」标识
	conjMask = ^entryID(0xF)
)

// makeConjID 组装定向包标识（incl 位恒为 0）
func makeConjID(ruleID uint64, conjIdx, size int) entryID {
	return entryID(size)<<sizeShift |
		entryID(ruleID)<<ruleIDShift |
		entryID(conjIdx)<<conjIdxShift
}

// conjID 是 entryID 去掉极性后的定向包标识
func (e entryID) conjID() entryID { return e & conjMask }

// nextConj 返回「跳过本定向包全部记录」后的位置，即 (conjID|incl) + 1
func (e entryID) nextConj() entryID { return (e.conjID() | inclBit) + 1 }

func (e entryID) ruleID() uint64 { return uint64(e>>ruleIDShift) & maxRuleID }

func (e entryID) size() int { return int(e >> sizeShift) }

func (e entryID) isInclude() bool { return e&inclBit != 0 }
