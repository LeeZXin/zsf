package boolindex

import "math"

// Field 是定向字段名，对应画像里的一个维度（地域、兴趣、设备等）。
type Field string

// Condition 描述单个字段上的枚举定向条件。
//
// In 是正向条件，NotIn 是排除条件。二者与「画像里有没有这个字段」组合出的真值语义是：
//
//	In 非空   且画像缺该字段 -> 不满足
//	In 非空   且画像该字段取值与 In 无交集 -> 不满足
//	In 非空   且画像该字段取值与 In 有交集 -> 满足
//	NotIn 非空 且画像缺该字段 -> 满足（空交集，这是最容易记反的一条）
//	NotIn 非空 且画像该字段取值与 NotIn 无交集 -> 满足
//	NotIn 非空 且画像该字段取值与 NotIn 有交集 -> 不满足
//
// 同一字段同时命中 In 与 NotIn 时排除优先：NotIn 判负，不再看 In。
// 与 In/NotIn 里出现重复取值等价，重复值会被忽略。
//
// 数值范围条件与枚举条件互斥：同一个字段要么按枚举匹配，要么按数值范围匹配，
// 混用会在 AddRule 报错。数值范围只支持正向条件，不支持「不在某个区间内」——
// 那类需求请改用枚举字段。
//
// Condition 没有导出字段，只能由构造器产生：
//
//	枚举    In("bj", "sh") / NotIn("competitor")，追加用 WithIn / WithNotIn
//	数值范围 Between(25, 40) / GreaterThan(5) / AtLeast(25) / LessThan(40) / AtMost(40)
//
// 这么设计是因为「一个字段上同时挂枚举和范围」「区间的开闭记反」这类错误，
// 光靠 AddRule 校验抓不住（写出来都是合法的、都能建索引，只是投歪了），
// 只有让它们无从表达才真正安全。要读回内容用 Range 和 Enum。
type Condition struct {
	in    []string
	notIn []string
	num   *numberRange
}

// numberRange 是一条数值区间，lo/hi 是归一化后的闭区间边界。
//
// 不变式：lo > hi 当且仅当这条区间为空（永不命中）。空区间来自两种情况——
// 开区间落到 int64 两端之外（> MaxInt64 / < MinInt64），以及下界大于上界
// （Between(10, 5)，或链式收紧把区间收没了）。AddRule 会拒绝空区间，
// 所以它在索引里不存在，只在构造阶段短暂出现。
type numberRange struct {
	lo, hi int64
}

// emptyRange 是空区间的规范写法
var emptyRange = numberRange{lo: 1, hi: 0}

// newNumberRange 把「边界 + 开闭标志」归一成闭区间。
// 开区间靠把边界挪一格实现，两端各自处理溢出，溢出即空区间。
func newNumberRange(min, max *int64, minExclusive, maxExclusive bool) numberRange {
	lo, hi := int64(math.MinInt64), int64(maxRangeValue)
	if min != nil {
		switch {
		case minExclusive && *min == maxRangeValue:
			return emptyRange // > int64 上界
		case minExclusive:
			lo = *min + 1
		default:
			lo = *min
		}
	}
	if max != nil {
		switch {
		case maxExclusive && *max == math.MinInt64:
			return emptyRange // < int64 下界
		case maxExclusive:
			hi = *max - 1
		default:
			hi = *max
		}
	}
	return numberRange{lo: lo, hi: hi}
}

func rangeOf(min, max int64) *numberRange {
	r := newNumberRange(&min, &max, false, false)
	return &r
}

func openRangeOf(v int64, asMin bool) *numberRange {
	var r numberRange
	if asMin {
		r = newNumberRange(&v, nil, true, false)
	} else {
		r = newNumberRange(nil, &v, false, true)
	}
	return &r
}

func halfRangeOf(v int64, asMin bool) *numberRange {
	var r numberRange
	if asMin {
		r = newNumberRange(&v, nil, false, false)
	} else {
		r = newNumberRange(nil, &v, false, false)
	}
	return &r
}

// In 构造只有正向条件的枚举定向：画像该字段的取值落在 values 里才算满足。
func In(values ...string) Condition {
	return Condition{in: values}
}

// NotIn 构造只有排除条件的枚举定向：画像该字段的取值落在 values 里即判不满足。
//
// 注意单独使用 NotIn 的定向包等价于「命中所有人，除了带上被排除值的请求」，
// 它不带任何正向约束，画像缺该字段时同样命中。要限定范围必须配合 In。
func NotIn(values ...string) Condition {
	return Condition{notIn: values}
}

// Between 构造闭区间范围条件：画像该字段的数值落在 [min, max] 内才算命中。
func Between(min, max int64) Condition {
	return Condition{num: rangeOf(min, max)}
}

// AtLeast 构造 [v, +∞) 的范围条件。
func AtLeast(v int64) Condition {
	return Condition{num: halfRangeOf(v, true)}
}

// AtMost 构造 (-∞, v] 的范围条件。
func AtMost(v int64) Condition {
	return Condition{num: halfRangeOf(v, false)}
}

// GreaterThan 构造 (v, +∞) 的范围条件。
func GreaterThan(v int64) Condition {
	return Condition{num: openRangeOf(v, true)}
}

// LessThan 构造 (-∞, v) 的范围条件。
func LessThan(v int64) Condition {
	return Condition{num: openRangeOf(v, false)}
}

// WithMin 在已有条件上收紧下界，返回新条件。
// 新下界比原来的更松时取原来那个——「收紧」就是求交集，不是替换。
func (c Condition) WithMin(v int64, exclusive bool) Condition {
	lo, hi := int64(math.MinInt64), int64(maxRangeValue)
	if c.num != nil {
		lo, hi = c.num.lo, c.num.hi
	}
	r := newNumberRange(&v, &hi, exclusive, false)
	r.lo = max(r.lo, lo)
	c.num = &r
	return c
}

// WithMax 在已有条件上收紧上界，返回新条件。
// 新上界比原来的更松时取原来那个。
func (c Condition) WithMax(v int64, exclusive bool) Condition {
	lo, hi := int64(math.MinInt64), int64(maxRangeValue)
	if c.num != nil {
		lo, hi = c.num.lo, c.num.hi
	}
	r := newNumberRange(&lo, &v, false, exclusive)
	r.hi = min(r.hi, hi)
	c.num = &r
	return c
}

// WithIn 在已有条件上追加正向取值，返回新条件（不修改原条件）。
func (c Condition) WithIn(values ...string) Condition {
	merged := make([]string, 0, len(c.in)+len(values))
	merged = append(merged, c.in...)
	merged = append(merged, values...)
	c.in = merged
	return c
}

// WithNotIn 在已有条件上追加排除取值，返回新条件（不修改原条件）。
func (c Condition) WithNotIn(values ...string) Condition {
	merged := make([]string, 0, len(c.notIn)+len(values))
	merged = append(merged, c.notIn...)
	merged = append(merged, values...)
	c.notIn = merged
	return c
}

// hasInclude 表示该字段是否提供正向条件。
//
// 这个判断决定了定向包的 size（见 entryID 的注释），也决定了画像缺该字段时是否判负，
// 是整套语义的支点，不要改成「In 与 NotIn 任一非空」。
func (c Condition) hasInclude() bool { return len(c.in) > 0 }

func (c Condition) hasExclude() bool { return len(c.notIn) > 0 }

// hasEnum 表示该字段用的是枚举条件
func (c Condition) hasEnum() bool { return len(c.in) > 0 || len(c.notIn) > 0 }

// hasRange 表示该字段用的是数值范围条件
func (c Condition) hasRange() bool { return c.num != nil }

// closedRange 返回归一化后的闭区间 [lo, hi]，empty 表示区间为空（永不命中）。
func (c Condition) closedRange() (lo, hi int64, empty bool) {
	if c.num == nil {
		return 0, 0, true
	}
	return c.num.lo, c.num.hi, c.num.lo > c.num.hi
}

// Enum 返回枚举条件的取值集合：in 是正向取值，notIn 是排除取值。
// ok 为 false 表示这个条件不是枚举条件（是数值范围条件或空条件）。
//
// 返回的切片是条件内部的切片，调用方不要修改它。
func (c Condition) Enum() (in, notIn []string, ok bool) {
	if !c.hasEnum() {
		return nil, nil, false
	}
	return c.in, c.notIn, true
}

// Range 返回数值范围条件的有效区间 [min, max]（闭区间）。
// ok 为 false 表示这个条件不是数值范围条件（是枚举条件或空条件）。
//
// 返回的是归一化之后的实际生效边界，不是构造时写的那两个数：
// GreaterThan(5) 得到 [6, MaxInt64]，LessThan(40) 得到 [MinInt64, 39]。
// 所以 MinInt64 / MaxInt64 就表示该侧无界。
// min > max 表示区间为空——AddRule 会拒绝这样的条件，只有链式收紧把它收没了才会见到。
func (c Condition) Range() (min, max int64, ok bool) {
	if c.num == nil {
		return 0, 0, false
	}
	return c.num.lo, c.num.hi, true
}

// Targeting 是一个定向包：字段到条件的映射，字段之间是「且」。
//
// 一条规则（广告）可以有多个定向包，包之间是「或」——任一包满足即命中。
// 空的 Targeting 会被 AddRule 拒绝：它的语义是命中所有人，在投放场景几乎总是配置事故。
type Targeting map[Field]Condition

// Profile 是一次检索输入（用户画像）：字段到取值的映射，同一字段的多个取值之间是「或」。
//
// 枚举字段的取值必须与建索引时用的是同一套字符串规范（大小写、空白都算），
// 本包只做精确匹配，不做归一化——归一化的责任在调用方，两侧必须一致。
//
// 数值字段的取值要传十进制整数（前后空白会被忽略）。解析失败时该取值什么都不命中，
// Match 不会报错，所以画像构造侧必须保证数值字段传的是合法整数，否则会静默不投放。
// 小数请由调用方自行放大成整数（例如金额乘以 100）。
type Profile map[Field][]string
