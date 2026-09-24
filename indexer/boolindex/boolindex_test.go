package boolindex

import (
	"errors"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// TestSemantics 是语义基线：排除条件在画像缺字段时判为满足（空交集），
// 正向条件在画像缺字段时判为不满足。这两条决定了「画像不传某字段」时的行为，
// 也是整个包最容易记反的地方。
//
// 用例与 be_indexer（VLDB09 实现的第三方库）实测结果逐条对齐。
func TestSemantics(t *testing.T) {
	ix := NewIndexer()

	rules := []struct {
		id   uint64
		pack Targeting
	}{
		{1, Targeting{"A": In("p")}},
		{2, Targeting{"A": NotIn("x")}},
		{3, Targeting{"A": In("p"), "B": NotIn("x")}},
		{4, Targeting{"A": NotIn("x"), "B": NotIn("y")}},
	}
	for _, r := range rules {
		if err := ix.AddRule(r.id, r.pack); err != nil {
			t.Fatalf("AddRule(%d) = %v, want nil", r.id, err)
		}
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	cases := []struct {
		name string
		p    Profile
		want []uint64
	}{
		{"空画像", nil, []uint64{2, 4}},
		{"A=p", Profile{"A": {"p"}}, []uint64{1, 2, 3, 4}},
		{"A=x 命中被排除值", Profile{"A": {"x"}}, []uint64{}},
		{"A=p,B=x 排除优先", Profile{"A": {"p"}, "B": {"x"}}, []uint64{1, 2, 4}},
		{"A=p,B=z", Profile{"A": {"p"}, "B": {"z"}}, []uint64{1, 2, 3, 4}},
		{"只给B=y 缺A", Profile{"B": {"y"}}, []uint64{2}},
		{"A=other", Profile{"A": {"other"}}, []uint64{2, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ix.Match(c.p)
			if !slices.Equal(got, c.want) {
				t.Fatalf("Match(%v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

// TestMultiValueNoFalsePositive 回归用例：定向包要求两个字段，画像只给其中一个字段
// （但给了该字段的两个取值），不得命中。
//
// 归并时若把「字段×取值」铺平成游标，同一字段的两个取值会被当成两个字段，
// 凑够 need 而误判命中。
func TestMultiValueNoFalsePositive(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(7, Targeting{
		"tag":  In("a", "x"),
		"city": In("bj"),
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
		{"只给 tag 的两个取值", Profile{"tag": {"a", "x"}}, []uint64{}},
		{"tag 取值重复", Profile{"tag": {"a", "a", "x"}}, []uint64{}},
		{"两个字段都满足", Profile{"tag": {"a"}, "city": {"bj"}}, []uint64{7}},
		{"两个字段都满足且多值", Profile{"tag": {"a", "x"}, "city": {"bj"}}, []uint64{7}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ix.Match(c.p); !slices.Equal(got, c.want) {
				t.Fatalf("Match(%v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

// TestPacksOrAndDedupe 一条规则的多个定向包之间是「或」，命中多次只返回一次。
func TestPacksOrAndDedupe(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(100,
		Targeting{"city": In("bj")},
		Targeting{"city": In("sh")},
	); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	if got := ix.Match(Profile{"city": {"bj"}}); !slices.Equal(got, []uint64{100}) {
		t.Fatalf("只命中第一个包: got=%v, want=[100]", got)
	}
	// 两个包同时命中，结果必须去重
	if got := ix.Match(Profile{"city": {"bj", "sh"}}); !slices.Equal(got, []uint64{100}) {
		t.Fatalf("两个包同时命中应去重: got=%v, want=[100]", got)
	}
	if got := ix.Match(Profile{"city": {"gz"}}); len(got) != 0 {
		t.Fatalf("都不命中: got=%v, want=[]", got)
	}
}

// TestUnconditionalPack 只带排除条件的定向包会命中所有请求，除非画像带上被排除的取值。
func TestUnconditionalPack(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(5, Targeting{"tag": NotIn("competitor")}); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	if got := ix.Match(nil); !slices.Equal(got, []uint64{5}) {
		t.Fatalf("空画像应命中: got=%v, want=[5]", got)
	}
	if got := ix.Match(Profile{"tag": {"beauty"}}); !slices.Equal(got, []uint64{5}) {
		t.Fatalf("未带被排除值应命中: got=%v, want=[5]", got)
	}
	if got := ix.Match(Profile{"tag": {"competitor"}}); len(got) != 0 {
		t.Fatalf("带上被排除值不应命中: got=%v, want=[]", got)
	}
	if s := ix.Stats(); s.Unconditional != 1 {
		t.Fatalf("Stats().Unconditional = %d, want 1", s.Unconditional)
	}
}

func TestAddRuleValidation(t *testing.T) {
	cases := []struct {
		name string
		id   uint64
		pack Targeting
	}{
		{"空定向包", 1, Targeting{}},
		{"字段名空", 1, Targeting{"": In("a")}},
		{"条件既无 In 也无 NotIn", 1, Targeting{"f": {}}},
		{"空取值", 1, Targeting{"f": In("")}},
		{"空取值 NotIn", 1, Targeting{"f": NotIn("")}},
		{"ruleID 超上限", maxRuleID + 1, Targeting{"f": In("a")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ix := NewIndexer()
			if err := ix.AddRule(c.id, c.pack); err == nil {
				t.Fatalf("AddRule() = nil, want error")
			}
			// 校验失败不得留下任何记录
			if s := ix.Stats(); s.Rules != 0 || s.Packs != 0 || s.Records != 0 {
				t.Fatalf("校验失败后索引被污染: %+v", s)
			}
		})
	}
}

func TestAddRuleBoundaries(t *testing.T) {
	t.Run("ruleID 上限可用", func(t *testing.T) {
		ix := NewIndexer()
		if err := ix.AddRule(maxRuleID, Targeting{"f": In("a")}); err != nil {
			t.Fatalf("AddRule(maxRuleID) = %v, want nil", err)
		}
		if err := ix.Build(); err != nil {
			t.Fatalf("Build() = %v, want nil", err)
		}
		if got := ix.Match(Profile{"f": {"a"}}); !slices.Equal(got, []uint64{maxRuleID}) {
			t.Fatalf("got=%v, want=[%d]", got, maxRuleID)
		}
	})

	t.Run("定向包数量上限", func(t *testing.T) {
		ix := NewIndexer()
		packs := make([]Targeting, maxConjIdx+1)
		for i := range packs {
			packs[i] = Targeting{"f": In("a")}
		}
		if err := ix.AddRule(1, packs...); err != nil {
			t.Fatalf("AddRule(%d packs) = %v, want nil", len(packs), err)
		}
		if err := ix.AddRule(2, append(packs, Targeting{"f": In("a")})...); err == nil {
			t.Fatal("AddRule with too many packs = nil, want error")
		}
	})

	t.Run("正向字段数上限", func(t *testing.T) {
		ix := NewIndexer()
		pack := Targeting{}
		for i := 0; i <= maxSize; i++ {
			pack[Field(string(rune('a'+i%26))+string(rune('0'+i/26)))] = In("v")
		}
		if err := ix.AddRule(1, pack); err == nil {
			t.Fatal("AddRule with too many fields = nil, want error")
		}
	})

	t.Run("重复 ruleID", func(t *testing.T) {
		ix := NewIndexer()
		if err := ix.AddRule(1, Targeting{"f": In("a")}); err != nil {
			t.Fatalf("第一次 AddRule = %v, want nil", err)
		}
		if err := ix.AddRule(1, Targeting{"f": In("b")}); err == nil {
			t.Fatal("重复 ruleID 应报错")
		}
	})
}

func TestLifecycle(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(1, Targeting{"f": In("a")}); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	// Build 幂等
	if err := ix.Build(); err != nil {
		t.Fatalf("重复 Build() = %v, want nil", err)
	}
	// Build 之后不可修改
	if err := ix.AddRule(2, Targeting{"f": In("b")}); !errors.Is(err, ErrBuilt) {
		t.Fatalf("Build 后 AddRule = %v, want ErrBuilt", err)
	}

	// 未 Build 时 Match 是编程错误，fail-fast
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("未 Build 就 Match 应当 panic")
			}
		}()
		NewIndexer().Match(nil)
	}()
}

func TestMatchIntoAppends(t *testing.T) {
	ix := NewIndexer()
	if err := ix.AddRule(1, Targeting{"f": In("a")}); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.AddRule(2, Targeting{"f": In("b")}); err != nil {
		t.Fatalf("AddRule() = %v, want nil", err)
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	dst := []uint64{99}
	dst = ix.MatchInto(Profile{"f": {"a"}}, dst)
	if !slices.Equal(dst, []uint64{99, 1}) {
		t.Fatalf("MatchInto 应追加且保留原内容: got=%v, want=[99 1]", dst)
	}
	dst = ix.MatchInto(Profile{"f": {"b"}}, dst)
	if !slices.Equal(dst, []uint64{99, 1, 2}) {
		t.Fatalf("第二次 MatchInto: got=%v, want=[99 1 2]", dst)
	}
}

// TestEntryIDLayout 断言位布局的三条性质，以及「+1 跳过整个定向包不会回绕」。
func TestEntryIDLayout(t *testing.T) {
	// 同一定向包的排除记录必须排在正向记录之前
	conj := makeConjID(12345, 7, 3)
	if !(conj < conj|inclBit) {
		t.Fatalf("排除记录应排在正向记录之前: conj=%d, incl=%d", conj, conj|inclBit)
	}
	// +1 跳过本定向包的全部记录：本包只有 conjID 与 conjID+1 两个槽，
	// 所以落在 conjID+2 即可，不必正好等于相邻定向包（相邻包差 16）。
	next := (conj | inclBit).nextConj()
	if next != conj+2 {
		t.Fatalf("nextConj() = %d, want %d", next, conj+2)
	}
	if !(next > conj|inclBit && next < makeConjID(12345, 8, 3)) {
		t.Fatalf("nextConj() = %d 必须越过本包且不越过相邻包 [%d, %d)",
			next, conj|inclBit, makeConjID(12345, 8, 3))
	}
	// 解码往返
	e := conj | inclBit
	if e.ruleID() != 12345 || e.size() != 3 || !e.isInclude() || e.conjID() != conj {
		t.Fatalf("解码错误: ruleID=%d size=%d incl=%v conj=%d", e.ruleID(), e.size(), e.isInclude(), e.conjID())
	}
	// 边界：最大合法 entryID 必须严格小于哨兵，否则 +1 会回绕成 0 导致死循环
	top := makeConjID(maxRuleID, maxConjIdx, maxSize) | inclBit
	if top >= nullEntry {
		t.Fatalf("最大合法 entryID = %#x, 必须小于 nullEntry", uint64(top))
	}
	if top+1 >= nullEntry {
		t.Fatalf("最大合法 entryID+1 = %#x, 必须仍小于 nullEntry", uint64(top+1))
	}
	if got, want := top.nextConj(), top.conjID()+2; got != want || got >= nullEntry {
		t.Fatalf("nextConj() = %#x, want %#x (且必须小于 nullEntry)", uint64(got), uint64(want))
	}
}

// TestSkipTo 用 sort.Search 作基准对拍跳表，覆盖空链、单元素、重复值、越界与幂等。
func TestSkipTo(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	targets := []entryID{0, 1, 2, 500, 999, 1000, 1 << 40, nullEntry}

	for range 300 {
		n := rnd.Intn(64)
		list := make(entries, n)
		for i := range list {
			list[i] = entryID(rnd.Intn(1000))
		}
		slices.Sort(list)
		for _, target := range targets {
			c := newCursor(list)
			got := c.skipTo(target)
			want := nullEntry
			if idx := sort.Search(n, func(i int) bool { return list[i] >= target }); idx < n {
				want = list[idx]
			}
			if got != want {
				t.Fatalf("skipTo(%d) on %v = %d, want %d", target, list, got, want)
			}
			if again := c.skipTo(target); again != want {
				t.Fatalf("skipTo 不幂等: 第一次 %d, 第二次 %d", want, again)
			}
		}
	}

	// 目标递增时位置只能前进，否则归并会死循环
	list := entries{}
	for range 100 {
		list = append(list, entryID(rnd.Intn(1<<20)))
	}
	slices.Sort(list)
	c := newCursor(list)
	prev := 0
	for _, target := range []entryID{1, 100, 5000, 100000, 1 << 19} {
		c.skipTo(target)
		if c.pos < prev {
			t.Fatalf("游标回退: target=%d pos=%d prev=%d", target, c.pos, prev)
		}
		prev = c.pos
	}
}
