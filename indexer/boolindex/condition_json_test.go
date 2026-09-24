package boolindex

import (
	"encoding/json"
	"slices"
	"testing"
)

// sameCondition 比较两个条件的语义：Condition 不可反射，只能走公开访问器比。
func sameCondition(a, b Condition) bool {
	aIn, aNotIn, aEnum := a.Enum()
	bIn, bNotIn, bEnum := b.Enum()
	if aEnum != bEnum || !slices.Equal(aIn, bIn) || !slices.Equal(aNotIn, bNotIn) {
		return false
	}
	aLo, aHi, aRange := a.Range()
	bLo, bHi, bRange := b.Range()
	if aRange != bRange {
		return false
	}
	return !aRange || (aLo == bLo && aHi == bHi)
}

func TestConditionJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		cond Condition
		want string // 期望的线格式
	}{
		{"正向枚举", In("bj", "sh"), `{"in":["bj","sh"]}`},
		{"排除枚举", NotIn("x"), `{"notIn":["x"]}`},
		{"双向枚举", In("a").WithNotIn("x"), `{"in":["a"],"notIn":["x"]}`},
		{"闭区间", Between(25, 40), `{"gte":25,"lte":40}`},
		{"单点区间", Between(7, 7), `{"gte":7,"lte":7}`},
		// 开区间归一化后落在整数上，编码成闭区间；GreaterThan(5) 与 AtLeast(6) 同形
		{"开下界归一成闭", GreaterThan(5), `{"gte":6}`},
		{"闭下界", AtLeast(6), `{"gte":6}`},
		{"开上界归一成闭", LessThan(40), `{"lte":39}`},
		{"闭上界", AtMost(40), `{"lte":40}`},
		{"负数区间", Between(-20, -10), `{"gte":-20,"lte":-10}`},
		{"枚举加区间", In("a").WithMin(5, false), `{"in":["a"],"gte":5}`},
		{"空条件", Condition{}, `{}`},
		// 两侧都无界必须显式写出边界，否则会被编码成 {} 而在往返中丢掉范围语义
		{"全域区间", AtLeast(minI64), `{"gte":-9223372036854775808,"lte":9223372036854775807}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := json.Marshal(c.cond)
			if err != nil {
				t.Fatalf("Marshal() = %v, want nil", err)
			}
			if string(data) != c.want {
				t.Fatalf("Marshal() = %s, want %s", data, c.want)
			}

			var back Condition
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatalf("Unmarshal(%s) = %v, want nil", data, err)
			}
			if !sameCondition(back, c.cond) {
				t.Fatalf("往返后语义变了: 原 %+v -> 解出 %+v", c.cond, back)
			}
		})
	}
}

// TestConditionJSONFromHandWritten 手写配置（用 gt/lt 表达开区间）也要能解。
func TestConditionJSONFromHandWritten(t *testing.T) {
	cases := []struct {
		json string
		want Condition
	}{
		{`{"gt":5}`, GreaterThan(5)},
		{`{"lt":40}`, LessThan(40)},
		{`{"gte":25,"lte":40}`, Between(25, 40)},
		{`{"in":["bj"],"notIn":["x"]}`, In("bj").WithNotIn("x")},
		{`{}`, Condition{}},
		{`null`, Condition{}},
	}
	for _, c := range cases {
		t.Run(c.json, func(t *testing.T) {
			var got Condition
			if err := json.Unmarshal([]byte(c.json), &got); err != nil {
				t.Fatalf("Unmarshal(%s) = %v, want nil", c.json, err)
			}
			if !sameCondition(got, c.want) {
				t.Fatalf("Unmarshal(%s) = %+v, want %+v", c.json, got, c.want)
			}
		})
	}
}

func TestConditionJSONErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"gt 与 gte 同时出现", `{"gt":5,"gte":6}`},
		{"lt 与 lte 同时出现", `{"lt":5,"lte":6}`},
		{"未知键（not_in 写成了下划线）", `{"not_in":["x"]}`},
		{"未知键（打字错误）", `{"inn":["x"]}`},
		{"小数边界", `{"gte":25.5}`},
		{"字符串边界", `{"gte":"25"}`},
		{"类型不对", `{"in":"bj"}`},
		{"不是对象", `[1,2]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got Condition
			if err := json.Unmarshal([]byte(c.json), &got); err == nil {
				t.Fatalf("Unmarshal(%s) = nil, want error", c.json)
			}
		})
	}
}

// TestTargetingJSON 真正要用的形态：整份定向配置（字段 -> 条件）直接序列化/反序列化。
func TestTargetingJSON(t *testing.T) {
	orig := Targeting{
		"city": In("bj", "sh"),
		"tag":  NotIn("competitor"),
		"age":  Between(25, 40),
		"arpu": GreaterThan(500),
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal() = %v, want nil", err)
	}

	// 编解码出来的配置必须能直接建索引并给出同样的检索结果
	ix := NewIndexer()
	if err := ix.AddRule(1, orig); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	var back Targeting
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal(%s) = %v, want nil", data, err)
	}
	ix2 := NewIndexer()
	if err := ix2.AddRule(1, back); err != nil {
		t.Fatalf("反序列化后的配置 AddRule() = %v, want nil", err)
	}
	if err := ix2.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	profiles := []Profile{
		{"city": {"bj"}, "tag": {"beauty"}, "age": {"30"}, "arpu": {"800"}},
		{"city": {"bj"}, "tag": {"competitor"}, "age": {"30"}, "arpu": {"800"}},
		{"city": {"bj"}, "tag": {"beauty"}, "age": {"41"}, "arpu": {"800"}},
		{"city": {"bj"}, "tag": {"beauty"}, "age": {"30"}, "arpu": {"100"}},
		{"city": {"gz"}, "tag": {"beauty"}, "age": {"30"}, "arpu": {"800"}},
	}
	for _, p := range profiles {
		if !slices.Equal(ix.Match(p), ix2.Match(p)) {
			t.Fatalf("profile %v: 原索引 %v, 反序列化后 %v", p, ix.Match(p), ix2.Match(p))
		}
	}
}

// TestConditionJSONNullAndOmit 零值边界：省略的键、null、空数组都表示「没有这一类约束」。
func TestConditionJSONNullAndOmit(t *testing.T) {
	var got Condition
	if err := json.Unmarshal([]byte(`{"in":[],"notIn":null}`), &got); err != nil {
		t.Fatalf("Unmarshal() = %v, want nil", err)
	}
	if _, _, ok := got.Enum(); ok {
		t.Fatalf("空数组与 null 都应视作没有枚举约束, got %+v", got)
	}
	if _, _, ok := got.Range(); ok {
		t.Fatalf("没有边界键就不该是范围条件, got %+v", got)
	}
	// 这种条件本身不合法（既没有枚举也没有范围），由 AddRule 拒绝
	ix := NewIndexer()
	if err := ix.AddRule(1, Targeting{"f": got}); err == nil {
		t.Fatal("空条件应在 AddRule 被拒绝")
	}
}

// TestConditionJSONInt64Boundary int64 两端要能精确往返，不能被 float64 精度吃掉。
func TestConditionJSONInt64Boundary(t *testing.T) {
	for _, v := range []int64{0, 1, -1, maxI64, minI64, maxI64 - 1, minI64 + 1} {
		cond := GreaterThan(v - 1)
		data, err := json.Marshal(cond)
		if err != nil {
			t.Fatalf("Marshal(%d) = %v", v, err)
		}
		var back Condition
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("Unmarshal(%s) = %v", data, err)
		}
		if !sameCondition(back, cond) {
			t.Fatalf("v=%d 往返后语义变了: %s -> %+v", v, data, back)
		}
	}
	// 超出 int64 的边界要被拒绝，不能截断
	var got Condition
	if err := json.Unmarshal([]byte(`{"gte":9223372036854775808}`), &got); err == nil {
		t.Fatal("超出 int64 的边界应被拒绝")
	}
	if err := json.Unmarshal([]byte(`{"gte":9223372036854775807.5}`), &got); err == nil {
		t.Fatal("小数边界应被拒绝")
	}
}
