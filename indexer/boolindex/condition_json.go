package boolindex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// Condition 的 JSON 线格式与构造器一一对应：
//
//	{"in":["bj","sh"], "notIn":["x"], "gte":25, "lte":40}
//
// 四个边界键 gt/gte/lt/lte 里，gt 与 gte 不能同时出现，lt 与 lte 同理。
// 无界的那一侧直接省略，所以 GreaterThan(5) 编码成 {"gte":6}。
//
// 注意编解码的是**归一化之后的区间**，不是构造时写的那两个数：
// GreaterThan(5) 与 AtLeast(6) 编码结果完全相同——它们本来就是同一条约束。
//
// 解码拒绝未知键：键名配错（比如把 notIn 写成 not_in）会当场报错，
// 不会静默丢掉条件的一部分导致规则变宽。
//
// 条件本身是否合法（枚举与范围混用、区间为空、取值是空串）不在解码阶段判，
// 统一留给 AddRule——校验只有一个地方，避免两处规则漂移。
type conditionJSON struct {
	In    []string `json:"in,omitempty"`
	NotIn []string `json:"notIn,omitempty"`
	Gt    *int64   `json:"gt,omitempty"`
	Gte   *int64   `json:"gte,omitempty"`
	Lt    *int64   `json:"lt,omitempty"`
	Lte   *int64   `json:"lte,omitempty"`
}

// MarshalJSON 实现 json.Marshaler。
func (c Condition) MarshalJSON() ([]byte, error) {
	w := conditionJSON{In: c.in, NotIn: c.notIn}
	if c.num != nil {
		lo, hi := c.num.lo, c.num.hi

		// 两侧都无界时必须把两个边界都写出来，否则整个范围条件会编码成 {}，
		// 往返之后变成「没有任何约束」的空条件。
		if lo == math.MinInt64 && hi == int64(maxRangeValue) {
			l, h := lo, hi
			w.Gte, w.Lte = &l, &h
			return json.Marshal(w)
		}
		if lo > math.MinInt64 {
			w.Gte = &lo
		}
		if hi < int64(maxRangeValue) {
			w.Lte = &hi
		}
		// 空区间（链式收紧收没了，AddRule 会拒绝）照实写出边界，
		// 不能因为「反正是空的」就省略，否则往返会把它变成无约束条件
		if w.Gte == nil && w.Lte == nil {
			l, h := lo, hi
			w.Gte, w.Lte = &l, &h
		}
	}
	return json.Marshal(w)
}

// UnmarshalJSON 实现 json.Unmarshaler。
func (c *Condition) UnmarshalJSON(data []byte) error {
	var w conditionJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("boolindex: decode condition: %w", err)
	}

	if w.Gt != nil && w.Gte != nil {
		return errors.New("boolindex: condition cannot set both gt and gte")
	}
	if w.Lt != nil && w.Lte != nil {
		return errors.New("boolindex: condition cannot set both lt and lte")
	}

	out := Condition{in: w.In, notIn: w.NotIn}
	if w.Gt != nil || w.Gte != nil || w.Lt != nil || w.Lte != nil {
		min, minExclusive := w.Gte, false
		if w.Gt != nil {
			min, minExclusive = w.Gt, true
		}
		max, maxExclusive := w.Lte, false
		if w.Lt != nil {
			max, maxExclusive = w.Lt, true
		}
		r := newNumberRange(min, max, minExclusive, maxExclusive)
		out.num = &r
	}
	*c = out
	return nil
}
