package boolindex

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	maxI64 = math.MaxInt64
	minI64 = math.MinInt64
)

// 值域两端在画像里是字符串形式
var (
	maxI64Str = strconv.FormatInt(maxI64, 10)
	minI64Str = strconv.FormatInt(minI64, 10)
)

// bruteRangeMatch 按区间定义直接求值，作为范围检索的期望来源。
// 手算开闭区间边界极易出错，所以除关键边界外，期望值都由它推导。
func bruteRangeMatch(cond Condition, values []string) bool {
	lo, hi, empty := cond.closedRange()
	if empty {
		return false
	}
	for _, v := range values {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		if n >= lo && n <= hi {
			return true
		}
	}
	return false
}

// expectByOracle 用暴力求值算出这批规则在给定取值下的期望命中集合
func expectByOracle(rules map[uint64]Condition, value string) []uint64 {
	var want []uint64
	for id, cond := range rules {
		if bruteRangeMatch(cond, []string{value}) {
			want = append(want, id)
		}
	}
	slices.Sort(want)
	return want
}

func buildRangeIndex(t *testing.T, rules map[uint64]Condition) *Indexer {
	t.Helper()
	ix := NewIndexer()
	for id, cond := range rules {
		if err := ix.AddRule(id, Targeting{"n": cond}); err != nil {
			t.Fatalf("AddRule(%d, %+v) = %v, want nil", id, cond, err)
		}
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	return ix
}

// TestRangeSemantics 各范围构造器在边界前后的行为，期望值来自暴力求值。
func TestRangeSemantics(t *testing.T) {
	rules := map[uint64]Condition{
		1: Between(10, 20),   // [10,20]
		2: GreaterThan(30),   // (30,+inf)
		3: LessThan(0),       // (-inf,0)
		4: AtLeast(40),       // [40,+inf)
		5: AtMost(-10),       // (-inf,-10]
		6: Between(5, 5),     // 单点区间
		7: Between(-20, -10), // 负数区间
		8: Condition{},       // 空条件，永不应命中
	}
	// 第 8 条是空条件，AddRule 会拒绝，单独剔除后建索引
	delete(rules, 8)

	ix := buildRangeIndex(t, rules)

	values := []string{
		"9", "10", "11", "20", "21",
		"29", "30", "31", "39", "40", "41", "100",
		"-100", "-21", "-20", "-15", "-11", "-10", "-9", "-1", "0", "1",
		"5", "6",
		maxI64Str, minI64Str,
	}
	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			got := ix.Match(Profile{"n": {v}})
			want := expectByOracle(rules, v)
			if !slices.Equal(got, want) {
				t.Fatalf("Match(n=%s) = %v, want %v", v, got, want)
			}
		})
	}
}

// TestRangeBoundaries 端点是 int64 两端时不能溢出或丢结果。
func TestRangeBoundaries(t *testing.T) {
	rules := map[uint64]Condition{
		1: GreaterThan(maxI64 - 1),
		2: AtMost(maxI64),
		3: LessThan(minI64 + 1),
		4: AtLeast(minI64),
	}
	ix := buildRangeIndex(t, rules)

	for _, v := range []string{maxI64Str, strconv.FormatInt(maxI64-1, 10), minI64Str, strconv.FormatInt(minI64+1, 10), "0"} {
		t.Run(v, func(t *testing.T) {
			got := ix.Match(Profile{"n": {v}})
			want := expectByOracle(rules, v)
			if !slices.Equal(got, want) {
				t.Fatalf("Match(n=%s) = %v, want %v", v, got, want)
			}
		})
	}
	// 关键语义显式钉住：AtMost(MaxInt64) 必须覆盖 MaxInt64 本身
	if got := ix.Match(Profile{"n": {maxI64Str}}); !slices.Contains(got, uint64(2)) {
		t.Fatalf("AtMost(MaxInt64) 应命中 MaxInt64: got=%v", got)
	}
	// GreaterThan(MaxInt64-1) 只覆盖 MaxInt64
	if got := ix.Match(Profile{"n": {strconv.FormatInt(maxI64-1, 10)}}); slices.Contains(got, uint64(1)) {
		t.Fatalf("GreaterThan(MaxInt64-1) 不应命中 MaxInt64-1: got=%v", got)
	}
}

// TestRangeWithEnumFields 同一个定向包里数值字段与枚举字段是「与」的关系。
func TestRangeWithEnumFields(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(1, Targeting{
		"city": In("bj", "sh"),
		"age":  Between(25, 40),
	}); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	cases := []struct {
		name string
		p    Profile
		want []uint64
	}{
		{"两个字段都满足", Profile{"city": {"bj"}, "age": {"30"}}, []uint64{1}},
		{"数值越界", Profile{"city": {"bj"}, "age": {"41"}}, []uint64{}},
		{"枚举不满足", Profile{"city": {"gz"}, "age": {"30"}}, []uint64{}},
		{"缺数值字段", Profile{"city": {"bj"}}, []uint64{}},
		{"缺枚举字段", Profile{"age": {"30"}}, []uint64{}},
		{"多值里各有一个命中", Profile{"city": {"gz", "sh"}, "age": {"50", "30"}}, []uint64{1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ix.Match(c.p); !slices.Equal(got, c.want) {
				t.Fatalf("Match(%v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

// TestRangeMultipleRangesSameField 同一字段上多条不同区间的规则要各自命中，
// 并检查区间划分没有把倒排记录写串。
func TestRangeMultipleRangesSameField(t *testing.T) {
	rules := map[uint64]Condition{
		1: Between(0, 9),
		2: Between(10, 19),
		3: Between(20, 29),
		4: AtLeast(25),
		5: LessThan(0),
	}
	ix := buildRangeIndex(t, rules)

	values := []string{"-1", "0", "5", "9", "10", "15", "19", "20", "22", "24", "25", "29", "30", "100"}
	for _, v := range values {
		t.Run(v, func(t *testing.T) {
			got := ix.Match(Profile{"n": {v}})
			want := expectByOracle(rules, v)
			if !slices.Equal(got, want) {
				t.Fatalf("Match(n=%s) = %v, want %v", v, got, want)
			}
		})
	}

	s := ix.Stats()
	if s.NumberFields != 1 || s.RangeSegments == 0 {
		t.Fatalf("Stats() = %+v, want 1 个数值字段且区间数 > 0", s)
	}
}

// TestRangeInvalidValueIsNoMatch 数值字段传了非整数时该取值什么都不命中。
// 这是有意的设计：Match 在热路径上不返回 error，代价是画像构造出错时静默不投放，
// 所以画像侧必须自己保证数值字段传的是合法整数。
func TestRangeInvalidValueIsNoMatch(t *testing.T) {
	ix := buildRangeIndex(t, map[uint64]Condition{1: Between(0, 100)})

	for _, v := range []string{"abc", "12.5", "", "1e3", "0x10", "-"} {
		t.Run(v, func(t *testing.T) {
			if got := ix.Match(Profile{"n": {v}}); len(got) != 0 {
				t.Fatalf("Match(n=%q) = %v, want 空", v, got)
			}
		})
	}
	// 前后空白容忍
	if got := ix.Match(Profile{"n": {" 50 "}}); !slices.Equal(got, []uint64{1}) {
		t.Fatalf("前后空白应被忽略: got=%v, want=[1]", got)
	}
}

func TestRangeValidation(t *testing.T) {
	t.Run("同字段混用枚举与范围", func(t *testing.T) {
		ix := NewIndexer()
		// 链式方法仍能把区间挂到已有枚举条件上，所以这条校验不是死代码
		if err := ix.AddRule(1, Targeting{"f": In("a").WithMin(5, false)}); err == nil {
			t.Fatal("枚举与范围混用应报错")
		}
	})

	t.Run("空区间", func(t *testing.T) {
		for name, cond := range map[string]Condition{
			"下界大于上界":      Between(10, 5),
			"开区间落在单点":     GreaterThan(5).WithMax(5, false), // (5,5] 为空
			"大于 int64 上界": GreaterThan(maxI64),
			"小于 int64 下界": LessThan(minI64),
		} {
			ix := NewIndexer()
			if err := ix.AddRule(1, Targeting{"f": cond}); err == nil {
				t.Fatalf("%s 应报错", name)
			}
		}
	})

	t.Run("跨规则字段类型冲突", func(t *testing.T) {
		ix := NewIndexer()
		if err := ix.AddRule(1, Targeting{"f": Between(0, 10)}); err != nil {
			t.Fatalf("第一次 AddRule = %v, want nil", err)
		}
		if err := ix.AddRule(2, Targeting{"f": In("a")}); err == nil {
			t.Fatal("数值字段再当枚举用应报错")
		}
		if err := ix.AddRule(3, Targeting{"f": Between(5, 15)}); err != nil {
			t.Fatalf("同类型重复使用 = %v, want nil", err)
		}
	})

	t.Run("校验失败不污染索引", func(t *testing.T) {
		ix := NewIndexer()
		_ = ix.AddRule(1, Targeting{"f": Between(1, 2)})
		_ = ix.AddRule(2, Targeting{"f": In("a")}) // 失败
		if s := ix.Stats(); s.Rules != 1 {
			t.Fatalf("校验失败后 Stats().Rules = %d, want 1", s.Rules)
		}
		if err := ix.Build(); err != nil {
			t.Fatalf("Build() = %v, want nil", err)
		}
		if got := ix.Match(Profile{"f": {"2"}}); !slices.Equal(got, []uint64{1}) {
			t.Fatalf("got=%v, want=[1]", got)
		}
	})
}

// TestConditionRange Range() 给的是归一化后的生效边界，不是构造时写的那两个数。
func TestConditionRange(t *testing.T) {
	cases := []struct {
		name      string
		cond      Condition
		wantMin   int64
		wantMax   int64
		wantOK    bool
		wantEmpty bool
	}{
		{"闭区间", Between(25, 40), 25, 40, true, false},
		{"单点", Between(7, 7), 7, 7, true, false},
		{"开下界挪一格", GreaterThan(5), 6, maxI64, true, false},
		{"闭下界", AtLeast(5), 5, maxI64, true, false},
		{"开上界挪一格", LessThan(40), minI64, 39, true, false},
		{"闭上界", AtMost(40), minI64, 40, true, false},
		{"负数区间", Between(-20, -10), -20, -10, true, false},
		{"枚举条件不是范围", In("bj"), 0, 0, false, false},
		{"排除条件不是范围", NotIn("bj"), 0, 0, false, false},
		{"空条件不是范围", Condition{}, 0, 0, false, false},
		{"链式收紧", Between(25, 40).WithMin(30, false).WithMax(35, true), 30, 34, true, false},
		{"上界溢出为空", GreaterThan(maxI64), 0, 0, true, true},
		{"下界溢出为空", LessThan(minI64), 0, 0, true, true},
		{"下界大于上界为空", Between(10, 5), 0, 0, true, true},
		{"链式收没了为空", Between(25, 40).WithMin(50, false), 0, 0, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMin, gotMax, ok := c.cond.Range()
			if ok != c.wantOK {
				t.Fatalf("Range() ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if empty := gotMin > gotMax; empty != c.wantEmpty {
				t.Fatalf("Range() = [%d,%d] 判空=%v, want 判空=%v", gotMin, gotMax, empty, c.wantEmpty)
			}
			if !c.wantEmpty && (gotMin != c.wantMin || gotMax != c.wantMax) {
				t.Fatalf("Range() = [%d,%d], want [%d,%d]", gotMin, gotMax, c.wantMin, c.wantMax)
			}
		})
	}
}

// TestConditionEnum Enum() 与 Range() 对称：ok 表示「是不是这一类条件」，
// 不是「这条条件合不合法」——枚举与范围混用的条件两个访问器都会说 ok，
// 那种条件由 AddRule 拒绝。
func TestConditionEnum(t *testing.T) {
	cases := []struct {
		name      string
		cond      Condition
		wantIn    []string
		wantNotIn []string
		wantOK    bool
	}{
		{"正向枚举", In("a", "b"), []string{"a", "b"}, nil, true},
		{"排除枚举", NotIn("x"), nil, []string{"x"}, true},
		{"双向枚举", In("a").WithNotIn("x"), []string{"a"}, []string{"x"}, true},
		{"链式追加正向", In("a").WithIn("b"), []string{"a", "b"}, nil, true},
		{"数值范围不是枚举", Between(1, 2), nil, nil, false},
		{"空条件不是枚举", Condition{}, nil, nil, false},
		{"混用时两个访问器都报 ok", In("a").WithMin(5, false), []string{"a"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotIn, gotNotIn, ok := c.cond.Enum()
			if ok != c.wantOK {
				t.Fatalf("Enum() ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if !slices.Equal(gotIn, c.wantIn) || !slices.Equal(gotNotIn, c.wantNotIn) {
				t.Fatalf("Enum() = (%v, %v), want (%v, %v)", gotIn, gotNotIn, c.wantIn, c.wantNotIn)
			}
		})
	}
}

// TestConditionChaining 链式方法只收紧指定的一侧，另一侧边界必须保留。
// numberRange 存的是归一化后的闭区间，开闭信息已经烘进边界值里，
// 所以改写一侧不会丢掉另一侧的开闭语义。
func TestConditionChaining(t *testing.T) {
	cases := []struct {
		name   string
		cond   Condition
		wantLo int64
		wantHi int64
	}{
		{"收紧下界", Between(25, 40).WithMin(30, false), 30, 40},
		{"下界更松时不放大", Between(25, 40).WithMin(10, false), 25, 40},
		{"收紧上界含边界", Between(25, 40).WithMax(35, false), 25, 35},
		{"收紧上界不含边界", Between(25, 40).WithMax(35, true), 25, 34},
		{"上界更松时不放大", Between(25, 40).WithMax(99, false), 25, 40},
		{"开下界后收上界", GreaterThan(5).WithMax(10, true), 6, 9},
		{"开上界后收下界", LessThan(40).WithMin(30, false), 30, 39},
		{"无下界时设下界", AtMost(40).WithMin(30, true), 31, 40},
		{"连续收紧", AtLeast(1).WithMin(10, false).WithMin(20, false), 20, maxI64},
		{"链式后相交为空", Between(25, 40).WithMin(50, false), 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi, empty := c.cond.closedRange()
			if c.name == "链式后相交为空" {
				if !empty {
					t.Fatalf("closedRange() = [%d,%d], want empty", lo, hi)
				}
				return
			}
			if empty || lo != c.wantLo || hi != c.wantHi {
				t.Fatalf("closedRange() = [%d,%d] empty=%v, want [%d,%d]", lo, hi, empty, c.wantLo, c.wantHi)
			}
		})
	}
}

// TestCursorCacheInvariant 断言游标缓存值与 list[pos] 始终一致。
// 缓存是为了性能，但新增改动 pos 的代码路径时很容易忘记同步 cur。
func TestCursorCacheInvariant(t *testing.T) {
	check := func(c *cursor) {
		t.Helper()
		if c.pos >= len(c.list) {
			if c.cur != nullEntry {
				t.Fatalf("耗尽时 cur = %d, want nullEntry", c.cur)
			}
			return
		}
		if c.cur != c.list[c.pos] {
			t.Fatalf("缓存失效: cur = %d, list[%d] = %d", c.cur, c.pos, c.list[c.pos])
		}
	}

	list := entries{1, 3, 3, 7, 20, 21, 100}
	c := newCursor(list)
	check(&c)

	for _, target := range []entryID{0, 1, 2, 3, 4, 20, 21, 22, 99, 100, 101, nullEntry} {
		c.skipTo(target)
		check(&c)
	}
	for range 3 {
		c.skipTo(50)
		check(&c)
	}

	empty := newCursor(nil)
	check(&empty)
	empty.skipTo(1)
	check(&empty)
}
