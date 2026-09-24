package boolindex

import (
	"math"
	"slices"
	"sort"
)

// maxRangeValue 是数值字段值域的上界（含）。值域为 [math.MinInt64, math.MaxInt64]。
const maxRangeValue = math.MaxInt64

// pendingRange 是一条待建索引的数值范围记录，Build 期才落到区间划分上。
type pendingRange struct {
	lo, hi int64 // 闭区间
	eid    entryID
}

// numberSegment 是数轴上一段左闭右闭的区间，以及落在这段区间里的全部定向包记录。
type numberSegment struct {
	left, right int64
	entries     entries
}

// rangeIdx 是数值字段的区间索引：把数轴切成互不相交、首尾相接的区间，每段挂一条倒排链。
//
// 建索引时按所有范围端点切分（端点重复时不会重复切），所以区间数与规则数无关，
// 只与端点的去重数有关。查询时二分定位到所在区间，O(logN)。
//
// 代价在写入侧：一条范围记录会被写进它覆盖的每一个区间，所以「端点特别分散 + 大量
// 宽区间」会让倒排记录数放大。广告定向的区间通常是年龄/价格分段这类少量固定端点，
// 放大倍数很小；用 Stats().RangeSegments 可以观测区间数。
type rangeIdx struct {
	segments []numberSegment
}

// buildRangeIdx 用两遍法建索引：先收集全部端点切分数轴，再把每条记录写进它覆盖的区间。
//
// 不在插入时切分区间，是为了避开切片别名问题——切分出的相邻区间会共享同一段底层数组，
// 之后再往其中一段追加就会覆写另一段的内容。两遍法每个区间独立追加，不存在这个问题。
func buildRangeIdx(ranges []pendingRange) *rangeIdx {
	if len(ranges) == 0 {
		return nil
	}

	cuts := make([]int64, 0, len(ranges)*2+2)
	cuts = append(cuts, math.MinInt64)
	for _, r := range ranges {
		cuts = append(cuts, r.lo)
		// 右边界取 hi+1 让区间左闭右闭接得上；hi 已是上界时饱和，避免溢出
		if r.hi < maxRangeValue {
			cuts = append(cuts, r.hi+1)
		}
	}
	cuts = append(cuts, maxRangeValue)
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)

	// 每个切点起一段，段右端是下一个切点减一（左闭右闭）。
	// 段数等于切点数：最后一个切点已经是值域上界，它自己起最后一段 [上界, 上界]。
	// 少建这一段会把「覆盖到上界」和「只取值域上界」两种情况混成同一段。
	idx := &rangeIdx{segments: make([]numberSegment, 0, len(cuts))}
	for i := range cuts {
		seg := numberSegment{left: cuts[i], right: maxRangeValue}
		if i+1 < len(cuts) {
			seg.right = cuts[i+1] - 1
		}
		idx.segments = append(idx.segments, seg)
	}

	for _, r := range ranges {
		start := sort.Search(len(idx.segments), func(i int) bool {
			return idx.segments[i].right >= r.lo
		})
		for i := start; i < len(idx.segments) && idx.segments[i].left <= r.hi; i++ {
			idx.segments[i].entries = append(idx.segments[i].entries, r.eid)
		}
	}
	for i := range idx.segments {
		if len(idx.segments[i].entries) > 1 {
			slices.Sort(idx.segments[i].entries)
		}
	}
	return idx
}

// lookup 返回取值 v 所在区间的倒排链，v 落在任何区间之外时返回 nil。
func (idx *rangeIdx) lookup(v int64) entries {
	i := sort.Search(len(idx.segments), func(i int) bool {
		return idx.segments[i].right >= v
	})
	if i >= len(idx.segments) || idx.segments[i].left > v {
		return nil
	}
	return idx.segments[i].entries
}

// records 统计全部区间上的倒排记录数
func (idx *rangeIdx) records() (n int) {
	for i := range idx.segments {
		n += len(idx.segments[i].entries)
	}
	return n
}
