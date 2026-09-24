// Package boolindex 提供枚举与数值范围定向的反向检索：把百万级的定向规则常驻内存，
// 给定一份用户画像，一次求出所有满足条件的规则 ID。
//
// 包名取自「布尔表达式索引」（Boolean Expression Index）——每条规则带的是自己的布尔
// 条件，索引的用途是「拿一个请求去撞出所有条件为真的规则」，方向与搜索引擎相反。
//
// 适用形态是「规则带条件、请求带特征」的 DNF 检索（广告定向、规则引擎）：
//
//	规则 = 若干定向包取「或」；定向包 = 若干字段条件取「且」；
//	字段条件 = 取值落在 In 且不落在 NotIn（枚举），或落在某个数值区间（范围）
//
// 与搜索引擎的区别：搜索引擎是一个查询对全部文档求值，这里是每条规则带着自己的
// 条件对同一个请求求值，所以不能靠倒排求交直接得到答案——本包按 VLDB09 的
// 布尔表达式索引实现，用「定向包 size 分组 + 多路归并」来判定条件数各异的规则。
//
// 生命周期与并发：
//
//	ix := boolindex.NewIndexer()
//	_ = ix.AddRule(1001, pack)   // 构建期：可反复调用，全部校验通过才写入
//	_ = ix.Build()               // 编译：排序倒排链、建数值区间索引，此后索引只读
//	hits := ix.Match(profile)    // 查询期：可多 goroutine 并发调用
//
// 索引 Build 之后不可变：更新一律全量重建一份新的 Indexer，再原子替换指针
// （配合 zsf 的 cacheutil.Locked 或 atomic.Pointer）。单机百万级规则的全量重建
// 在秒级，比维护增量更新简单得多，也顺带避开了「半个定向包入库」导致的错投。
package boolindex

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ErrBuilt 表示索引已经 Build 过，不能再修改。
var ErrBuilt = errors.New("boolindex: index already built")

// fieldKind 是字段的匹配方式，由该字段第一次被使用时的条件类型决定，
// 同一个字段在后续规则里必须保持一致。
type fieldKind uint8

const (
	kindEnum   fieldKind = iota // 枚举：取值精确匹配
	kindNumber                  // 数值：区间匹配
)

// fieldHolder 是单个字段的倒排数据。
// 枚举字段用 entries，数值字段在 Build 期用 ranges 建成 idx。
type fieldHolder struct {
	kind    fieldKind
	entries map[string]entries
	ranges  []pendingRange
	idx     *rangeIdx
}

// list 把画像里该字段的一个取值翻译成倒排链。
//
// 数值字段要求画像取值是十进制整数（允许前后空白）。解析失败时返回 nil，
// 也就是「这个取值什么都不命中」，不会报错——Match 在热路径上不返回 error。
// 画像构造侧必须保证数值字段传的是合法整数，否则会静默不投放。
func (h *fieldHolder) list(v string) entries {
	if h.kind == kindEnum {
		return h.entries[v]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || h.idx == nil {
		return nil
	}
	return h.idx.lookup(n)
}

// Stats 是索引的规模统计，用于启动期自检与容量评估。
type Stats struct {
	Rules   int // 规则数
	Packs   int // 定向包数
	Fields  int // 出现过的字段数
	Records int // 倒排记录总数

	// NumberFields 是数值范围字段数，RangeSegments 是这些字段的区间总数。
	// 区间数只与范围端点的去重数有关；它明显偏大时说明端点很分散，
	// 宽区间会被写进很多区间，倒排记录数会放大，值得关注。
	NumberFields  int
	RangeSegments int

	// Unconditional 是没有正向条件的定向包数——这类包会命中所有请求，
	// 只有画像明确带上被排除的取值时才落选。业务上通常意味着「除竞品外全投」，
	// 也可能是漏配了正向条件，上线前值得核对一遍。
	Unconditional int
}

// Indexer 是定向索引。零值不可用，必须由 NewIndexer 构造。
type Indexer struct {
	built bool

	fields map[Field]*fieldHolder

	// unconditional 存放没有正向条件的定向包的正向记录。
	// 它们不挂在任何 (字段,取值) 上，归并时作为一个独立的游标组参与判定。
	unconditional entries

	ruleIDs map[uint64]struct{}
	stats   Stats
}

// NewIndexer 创建一个空索引。
func NewIndexer() *Indexer {
	return &Indexer{
		fields:  make(map[Field]*fieldHolder),
		ruleIDs: make(map[uint64]struct{}),
	}
}

// pendingRecord 是校验阶段产出的待写入记录。AddRule 先攒齐全部记录，
// 任一校验失败就整体放弃，避免出现「一个定向包只写进去一半条件」——
// 那会让本该被排除的请求命中，是静默错投。
type pendingRecord struct {
	field         Field
	value         string
	rng           pendingRange
	isRange       bool
	eid           entryID
	unconditional bool
}

// AddRule 登记一条规则。packs 是这条规则的定向包，包之间是「或」。
//
// 同一个 ruleID 只能调用一次；重复登记会被拒绝而不会覆盖。
// 全部校验通过后才会写入索引，失败时索引保持原样。
//
// 约定：Build 之后调用返回 ErrBuilt，不 panic（调用方可以走「重新建一份再替换」的路径）。
func (ix *Indexer) AddRule(id uint64, packs ...Targeting) error {
	if ix.built {
		return ErrBuilt
	}
	if id > maxRuleID {
		return fmt.Errorf("boolindex: ruleID=%d out of range [0, %d]", id, uint64(maxRuleID))
	}
	if _, dup := ix.ruleIDs[id]; dup {
		return fmt.Errorf("boolindex: ruleID=%d already added", id)
	}
	if len(packs) == 0 {
		return fmt.Errorf("boolindex: ruleID=%d has no targeting pack", id)
	}
	if len(packs) > maxConjIdx+1 {
		return fmt.Errorf("boolindex: ruleID=%d has %d packs, limit is %d", id, len(packs), maxConjIdx+1)
	}

	records := make([]pendingRecord, 0, 8*len(packs))
	for idx, pack := range packs {
		size, err := ix.validatePack(id, idx, pack)
		if err != nil {
			return err
		}
		conj := makeConjID(id, idx, size)
		for f, cond := range pack {
			if cond.hasRange() {
				lo, hi, _ := cond.closedRange()
				records = append(records, pendingRecord{
					field: f, isRange: true, rng: pendingRange{lo: lo, hi: hi, eid: conj | inclBit},
				})
				continue
			}
			in, notIn, _ := cond.Enum()
			for _, v := range distinctValues(in) {
				records = append(records, pendingRecord{field: f, value: v, eid: conj | inclBit})
			}
			for _, v := range distinctValues(notIn) {
				records = append(records, pendingRecord{field: f, value: v, eid: conj})
			}
		}
		if size == 0 {
			// 只有排除条件的定向包：靠这条正向记录在归并时被判定，
			// 画像带上被排除的取值时，字段倒排里的排除记录会先把它压掉。
			records = append(records, pendingRecord{eid: conj | inclBit, unconditional: true})
		}
		ix.stats.Packs++
	}

	for _, r := range records {
		if r.unconditional {
			ix.unconditional = append(ix.unconditional, r.eid)
			ix.stats.Unconditional++
			continue
		}
		h := ix.fields[r.field]
		if h == nil {
			// 建 holder 必须显式写 kind：fieldKind 的零值是 kindEnum，
			// 漏写会让数值字段的第一个 holder 被标成枚举，同字段的第二条范围规则就报错
			h = &fieldHolder{kind: kindEnum, entries: make(map[string]entries)}
			if r.isRange {
				h.kind, h.entries = kindNumber, nil
			}
			ix.fields[r.field] = h
		}
		if r.isRange {
			h.ranges = append(h.ranges, r.rng)
			continue
		}
		h.entries[r.value] = append(h.entries[r.value], r.eid)
	}

	ix.ruleIDs[id] = struct{}{}
	ix.stats.Rules++
	return nil
}

// validatePack 校验一个定向包并返回它的 size。
//
// size 的定义是「有正向条件的字段个数」，不是取值个数、也不是条件条数：
// 同一个字段写多个 In 取值只算一个字段，一条数值范围条件也算一个字段。
// 这个数字烙进每条记录的 entryID 高位，归并时用它判断「这个包要求几个字段同时满足」，
// 写错会导致漏投或错投。
//
// 校验过程中会顺带锁定字段的匹配方式（枚举/数值），后续规则必须保持一致。
func (ix *Indexer) validatePack(id uint64, idx int, pack Targeting) (int, error) {
	if len(pack) == 0 {
		return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] is empty, an empty pack matches every request", id, idx)
	}
	size := 0
	for f, cond := range pack {
		if f == "" {
			return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] has empty field name", id, idx)
		}
		kind, err := ix.validateCondition(id, idx, f, cond)
		if err != nil {
			return 0, err
		}
		if kind == kindEnum && cond.hasInclude() {
			size++
		} else if kind == kindNumber {
			size++ // 范围条件一定是正向条件
		}
	}
	if size > maxSize {
		return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] has %d fields with In, limit is %d", id, idx, size, maxSize)
	}
	return size, nil
}

func (ix *Indexer) validateCondition(id uint64, idx int, f Field, cond Condition) (fieldKind, error) {
	if cond.hasEnum() && cond.hasRange() {
		return 0, fmt.Errorf(
			"boolindex: ruleID=%d pack[%d] field=%s mixes enum (In/NotIn) with numeric range", id, idx, f)
	}

	var kind fieldKind
	switch {
	case cond.hasRange():
		kind = kindNumber
		if _, _, empty := cond.closedRange(); empty {
			return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] field=%s has an empty numeric range", id, idx, f)
		}
	case cond.hasEnum():
		kind = kindEnum
		in, notIn, _ := cond.Enum()
		if slices.Contains(append(append([]string{}, in...), notIn...), "") {
			return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] field=%s has empty value", id, idx, f)
		}
	default:
		return 0, fmt.Errorf("boolindex: ruleID=%d pack[%d] field=%s has neither In nor NotIn nor a range", id, idx, f)
	}

	// 同一个字段的匹配方式一旦确定就不能改：枚举用哈希表、数值用区间索引，
	// 混用会同时写坏两套结构
	if h := ix.fields[f]; h != nil && h.kind != kind {
		return 0, fmt.Errorf(
			"boolindex: ruleID=%d pack[%d] field=%s is already used as %s, cannot use it as %s",
			id, idx, f, h.kind, kind)
	}
	return kind, nil
}

func (k fieldKind) String() string {
	if k == kindNumber {
		return "numeric"
	}
	return "enum"
}

// distinctValues 去掉重复取值：重复值会在倒排链里写入两条相同记录，
// 虽然不影响结果（lower_bound 语义），但白占内存。
func distinctValues(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// Build 编译索引：排序枚举倒排链、建数值区间索引，之后索引进入只读状态。
//
// 目前总是返回 nil——全部校验都发生在 AddRule。保留 error 返回值是为了
// 将来加入全局约束（例如倒排记录总数上限）时不必改调用方代码。
// 重复调用是幂等的。
func (ix *Indexer) Build() error {
	if ix.built {
		return nil
	}
	records := len(ix.unconditional)
	for _, h := range ix.fields {
		switch h.kind {
		case kindNumber:
			h.idx = buildRangeIdx(h.ranges)
			h.ranges = nil
			if h.idx != nil {
				records += h.idx.records()
				ix.stats.RangeSegments += len(h.idx.segments)
				ix.stats.NumberFields++
			}
		default:
			for _, list := range h.entries {
				slices.Sort(list)
				records += len(list)
			}
		}
	}
	slices.Sort(ix.unconditional)

	ix.stats.Records = records
	ix.stats.Fields = len(ix.fields)
	ix.built = true

	// ruleIDs 只服务于 AddRule 的重复登记检测，百万级规则下这个集合本身要占几十 MB，
	// Build 之后不会再用（AddRule 已经拒绝继续写入），直接释放。
	// 重复登记必须拦住：同一个 ruleID 写两次会让两条规则的 conjIdx 撞在一起，
	// 索引记录互相污染，所以这个集合不能为了省内存去掉。
	ix.ruleIDs = nil
	return nil
}

// Stats 返回索引规模统计。Build 之前调用只反映已登记的记录数。
func (ix *Indexer) Stats() Stats { return ix.stats }

// Match 返回画像命中的全部 ruleID，按升序去重。
//
// 同一规则的多个定向包都命中时只返回一次。nil 画像等价于空画像，
// 此时只有不带正向条件的定向包会命中。
//
// 索引尚未 Build 时 panic：这是「忘了调用 Build」的编程错误，
// 静默返回空会让线上一条都不投，fail-fast 更安全。
func (ix *Indexer) Match(p Profile) []uint64 {
	if !ix.built {
		panic("boolindex: Match called before Build")
	}
	sc := acquireScratch()
	defer releaseScratch(sc)

	sc.ids = ix.match(p, sc, sc.ids[:0])
	ids := sc.ids
	slices.Sort(ids)
	ids = slices.Compact(ids)
	// 结果要交给调用方，这一次分配省不掉；scratch 里的缓冲靠池复用
	return slices.Clone(ids)
}

// MatchInto 与 Match 相同，但把结果追加到 dst 上（不覆盖已有内容），
// 便于在热路径上复用结果切片，做到零分配。只有本次追加的部分会被排序去重，
// dst 里原有内容的顺序不受影响。
func (ix *Indexer) MatchInto(p Profile, dst []uint64) []uint64 {
	if !ix.built {
		panic("boolindex: Match called before Build")
	}
	sc := acquireScratch()
	defer releaseScratch(sc)

	start := len(dst)
	dst = ix.match(p, sc, dst)

	// 归并按 size 递进，命中顺序是 (size, ruleID) 而不是 ruleID，且同一规则
	// 可能被多个定向包命中，这里统一排序去重得到稳定的升序结果。
	out := dst[start:]
	slices.Sort(out)
	out = slices.Compact(out)
	return dst[:start+len(out)]
}
