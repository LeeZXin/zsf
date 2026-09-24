package boolindex

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"
)

// buildIndex 用一组规则建全量索引，按 ruleID 升序建以保证可复现
func buildIndex(t *testing.T, rules map[uint64]Targeting) *Indexer {
	t.Helper()
	ix := NewIndexer()
	ids := make([]uint64, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if err := ix.AddRule(id, rules[id]); err != nil {
			t.Fatalf("AddRule(%d, %+v) = %v, want nil", id, rules[id], err)
		}
	}
	if err := ix.Build(); err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	return ix
}

// TestLayeredBasic 增量层里新增的规则与全量层的规则一起生效。
func TestLayeredBasic(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")},
		2: {"city": In("sh")},
	})
	db := NewDeltaBuilder()
	if err := db.Upsert(3, Targeting{"city": In("gz")}); err != nil {
		t.Fatalf("Upsert() = %v, want nil", err)
	}
	layered, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	cases := []struct {
		city string
		want []uint64
	}{
		{"bj", []uint64{1}},
		{"sh", []uint64{2}},
		{"gz", []uint64{3}},
		{"hz", []uint64{}},
	}
	for _, c := range cases {
		t.Run(c.city, func(t *testing.T) {
			if got := layered.Match(Profile{"city": {c.city}}); !slices.Equal(got, c.want) {
				t.Fatalf("Match(city=%s) = %v, want %v", c.city, got, c.want)
			}
		})
	}

	s := layered.Stats()
	if s.BaseRules != 2 || s.DeltaRules != 1 || s.Deleted != 0 {
		t.Fatalf("Stats() = %+v, want {Base:2 Delta:1 Deleted:0}", s)
	}
}

// TestLayeredUpdateMasking 这个用例是整个两层结构的关键：
// 广告改过定向之后，全量层里它的旧版本必须被完全遮蔽，
// 否则「旧配置命中、新配置不命中」的画像会被错误投放。
func TestLayeredUpdateMasking(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")}, // 旧版本只投北京
	})
	db := NewDeltaBuilder()
	// 新版本只投上海
	if err := db.Upsert(1, Targeting{"city": In("sh")}); err != nil {
		t.Fatalf("Upsert() = %v, want nil", err)
	}
	layered, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	cases := []struct {
		name string
		p    Profile
		want []uint64
	}{
		{"命中新版本", Profile{"city": {"sh"}}, []uint64{1}},
		{"旧版本命中但新版本不命中", Profile{"city": {"bj"}}, []uint64{}},
		{"两个都不命中", Profile{"city": {"gz"}}, []uint64{}},
		{"两个都命中只返回一次", Profile{"city": {"bj", "sh"}}, []uint64{1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := layered.Match(c.p); !slices.Equal(got, c.want) {
				t.Fatalf("Match(%v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

// TestLayeredDelete 删除靠「登记为接管但没有记录」实现，全量层里的旧结果被遮蔽。
func TestLayeredDelete(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")},
		2: {"city": In("bj")},
	})
	db := NewDeltaBuilder()
	db.Delete(1)
	layered, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}

	if got := layered.Match(Profile{"city": {"bj"}}); !slices.Equal(got, []uint64{2}) {
		t.Fatalf("Match() = %v, want [2]（1 已删除）", got)
	}
	if s := layered.Stats(); s.Deleted != 1 || s.DeltaRules != 0 {
		t.Fatalf("Stats() = %+v, want {Delta:0 Deleted:1}", s)
	}

	// 删除后又重新上线：墓碑要被摘掉
	if err := db.Upsert(1, Targeting{"city": In("bj")}); err != nil {
		t.Fatalf("Upsert() = %v, want nil", err)
	}
	layered, err = db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	if got := layered.Match(Profile{"city": {"bj"}}); !slices.Equal(got, []uint64{1, 2}) {
		t.Fatalf("重新上线后 Match() = %v, want [1 2]", got)
	}
}

// TestLayeredSkippedRuleKeepsBaseVersion 非法的新配置被跳过时，
// 广告保留全量层里的旧版本继续投放，同时错误被上报。
func TestLayeredSkippedRuleKeepsBaseVersion(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")},
	})
	db := NewDeltaBuilder()
	if err := db.Upsert(3, Targeting{"city": In("gz")}); err != nil {
		t.Fatalf("Upsert() = %v, want nil", err) // 合法
	}
	// 非法的更新：空定向包
	if err := db.Upsert(1, Targeting{"city": In("")}); err != nil {
		t.Fatalf("Upsert() = %v, want nil（合法性要到 Build 才判）", err)
	}

	layered, err := db.Build(base)
	var skipped *SkippedRulesError
	if !errors.As(err, &skipped) {
		t.Fatalf("Build() = %v, want *SkippedRulesError", err)
	}
	if len(skipped.Rules) != 1 || skipped.Rules[0].RuleID != 1 {
		t.Fatalf("被跳过的规则 = %+v, want ruleID=1", skipped.Rules)
	}
	if layered == nil {
		t.Fatal("Build() 即使有规则被跳过也必须返回可用快照")
	}
	if !strings.Contains(skipped.Error(), "[1]") {
		t.Fatalf("错误信息里应能看出是哪条规则: %s", skipped.Error())
	}

	// 旧版本继续生效，合法的新增也生效
	if got := layered.Match(Profile{"city": {"bj"}}); !slices.Equal(got, []uint64{1}) {
		t.Fatalf("被判非法的规则应保留旧版本: got=%v, want=[1]", got)
	}
	if got := layered.Match(Profile{"city": {"gz"}}); !slices.Equal(got, []uint64{3}) {
		t.Fatalf("合法的新增规则应生效: got=%v, want=[3]", got)
	}
}

func TestDeltaBuilderPending(t *testing.T) {
	db := NewDeltaBuilder()
	if upserts, deletes := db.Pending(); upserts != 0 || deletes != 0 {
		t.Fatalf("Pending() = (%d,%d), want (0,0)", upserts, deletes)
	}
	if err := db.Upsert(1, Targeting{"f": In("a")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	if err := db.Upsert(2, Targeting{"f": In("b")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	db.Delete(3)
	db.Delete(1) // 先更新后删除，应当只剩一条删除
	if upserts, deletes := db.Pending(); upserts != 1 || deletes != 2 {
		t.Fatalf("Pending() = (%d,%d), want (1,2)", upserts, deletes)
	}

	// Upsert 参数校验
	if err := db.Upsert(1); err == nil {
		t.Fatal("空 packs 应被拒绝")
	}
	if err := db.Upsert(maxRuleID+1, Targeting{"f": In("a")}); err == nil {
		t.Fatal("ruleID 越界应被拒绝")
	}
}

// TestLayeredBaseSwapDropsStaleDelta 换基线时，相对旧基线的增量必须自动作废。
//
// 这是整套分层方案唯一会造成静默错投的地方：增量层对全量层有遮蔽权，
// 基线换代之后还留着旧增量，就会用旧版本把新基线里已经改对的规则遮回去。
// 所以 Build 检测到 base 换了就直接清空，不指望调用方记得手动 Reset。
func TestLayeredBaseSwapDropsStaleDelta(t *testing.T) {
	base1 := buildIndex(t, map[uint64]Targeting{1: {"city": In("bj")}})

	db := NewDeltaBuilder()
	// 走增量登记：新增规则 2，当时配置是只投上海
	if err := db.Upsert(2, Targeting{"city": In("sh")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	snap1, err := db.Build(base1)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}
	if got := snap1.Match(Profile{"city": {"sh"}}); !slices.Equal(got, []uint64{2}) {
		t.Fatalf("换基线之前应命中: got=%v, want=[2]", got)
	}

	// 全量重建：库里规则 2 已经改成只投广州。
	// 这次改动没有经过 db（可能是别的实例改的、人工改库、或消息丢了）。
	base2 := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")},
		2: {"city": In("gz")},
	})

	// 同一个 db 直接 Build 新基线，不做任何显式清理
	snap2, err := db.Build(base2)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}
	if upserts, deletes := db.Pending(); upserts != 0 || deletes != 0 {
		t.Fatalf("换基线后旧增量应已作废: Pending()=(%d,%d)", upserts, deletes)
	}
	if got := snap2.Match(Profile{"city": {"sh"}}); len(got) != 0 {
		t.Fatalf("旧增量未作废，用旧配置投出去了: got=%v, want=[]", got)
	}
	if got := snap2.Match(Profile{"city": {"gz"}}); !slices.Equal(got, []uint64{2}) {
		t.Fatalf("新基线的配置被旧增量遮住了: got=%v, want=[2]", got)
	}
	if s := snap2.Stats(); s.DeltaRules != 0 || s.Deleted != 0 {
		t.Fatalf("Stats() = %+v, want 增量为空", s)
	}

	// 反过来也要成立：同一份基线重复 Build 不能把增量清掉，
	// 否则每次刷新快照都会把未并进全量的变更丢掉
	if err := db.Upsert(3, Targeting{"city": In("hz")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	for range 3 {
		snap, err := db.Build(base2)
		if err != nil {
			t.Fatalf("Build() = %v", err)
		}
		if got := snap.Match(Profile{"city": {"hz"}}); !slices.Equal(got, []uint64{3}) {
			t.Fatalf("同一份基线重复 Build 丢了增量: got=%v, want=[3]", got)
		}
	}
}

// TestDeltaBuilderRefresh Refresh 沿用当前基线，调用方不必自己存基线再传一遍。
func TestDeltaBuilderRefresh(t *testing.T) {
	db := NewDeltaBuilder()
	// 一次都没 Build 过时没有基线可用
	if _, err := db.Refresh(); err == nil {
		t.Fatal("还没有基线时 Refresh 应报错")
	}

	base := buildIndex(t, map[uint64]Targeting{1: {"city": In("bj")}})
	if err := db.Upsert(2, Targeting{"city": In("sh")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	if _, err := db.Build(base); err != nil {
		t.Fatalf("Build() = %v", err)
	}

	// 反复 Refresh：增量不能丢，也不能重复叠加
	for i := range 3 {
		snap, err := db.Refresh()
		if err != nil {
			t.Fatalf("第 %d 次 Refresh() = %v", i+1, err)
		}
		if got := snap.Match(Profile{"city": {"sh"}}); !slices.Equal(got, []uint64{2}) {
			t.Fatalf("第 %d 次 Refresh 后增量丢了: got=%v, want=[2]", i+1, got)
		}
		if s := snap.Stats(); s.DeltaRules != 1 {
			t.Fatalf("第 %d 次 Refresh 后 Stats() = %+v, want DeltaRules:1", i+1, s)
		}
	}

	// 换基线之后，Refresh 应当用新基线
	base2 := buildIndex(t, map[uint64]Targeting{
		1: {"city": In("bj")},
		2: {"city": In("gz")},
	})
	if _, err := db.Build(base2); err != nil {
		t.Fatalf("Build() = %v", err)
	}
	snap, err := db.Refresh()
	if err != nil {
		t.Fatalf("换基线后 Refresh() = %v", err)
	}
	// 旧增量已作废，以 base2 的配置为准
	if got := snap.Match(Profile{"city": {"gz"}}); !slices.Equal(got, []uint64{2}) {
		t.Fatalf("Refresh 没有用新基线: got=%v, want=[2]", got)
	}
	if got := snap.Match(Profile{"city": {"sh"}}); len(got) != 0 {
		t.Fatalf("旧增量应已作废: got=%v, want=[]", got)
	}
}

// TestLayeredMatchIntoAppends MatchInto 只处理本次追加的部分
func TestLayeredMatchIntoAppends(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{1: {"city": In("bj")}})
	db := NewDeltaBuilder()
	if err := db.Upsert(2, Targeting{"city": In("bj")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	layered, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	dst := []uint64{99}
	dst = layered.MatchInto(Profile{"city": {"bj"}}, dst)
	if !slices.Equal(dst, []uint64{99, 1, 2}) {
		t.Fatalf("MatchInto = %v, want [99 1 2]", dst)
	}
}

// TestLayeredDifferential 两层合并的结果必须与「把变更并进去之后重新建一份全量索引」
// 完全一致——这是这一层唯一的正确性标准。
func TestLayeredDifferential(t *testing.T) {
	rnd := rand.New(rand.NewSource(20240925))

	fields := []Field{"f0", "f1", "f2"}
	values := map[Field][]string{
		"f0": {"a", "b", "c"},
		"f1": {"x", "y"},
		"f2": {"p", "q", "r", "s"},
	}
	genTargeting := func() Targeting {
		pack := Targeting{}
		for _, f := range fields {
			r := rnd.Intn(10)
			vals := values[f]
			pick := func(n int) []string {
				out := make([]string, 0, n)
				for range n {
					out = append(out, vals[rnd.Intn(len(vals))])
				}
				return out
			}
			switch {
			case r < 4:
				pack[f] = In(pick(1 + rnd.Intn(2))...)
			case r < 5:
				pack[f] = In(pick(1)...).WithNotIn(pick(1)...)
			case r < 7:
				pack[f] = NotIn(pick(1)...)
			}
		}
		if len(pack) == 0 {
			pack[fields[rnd.Intn(len(fields))]] = In(values[fields[0]][0])
		}
		return pack
	}

	const total = 200
	// 前一半进全量层，后一半是「从来没有过的新增」
	baseRules := map[uint64]Targeting{}
	final := map[uint64]Targeting{}
	for i := 1; i <= total/2; i++ {
		id := uint64(i)
		tg := genTargeting()
		baseRules[id] = tg
		final[id] = tg
	}
	base := buildIndex(t, baseRules)

	db := NewDeltaBuilder()
	for _, id := range rnd.Perm(total) {
		ruleID := uint64(id + 1)
		if _, live := final[ruleID]; live && rnd.Intn(3) == 0 {
			db.Delete(ruleID) // 删除
			delete(final, ruleID)
			continue
		}
		tg := genTargeting()
		if err := db.Upsert(ruleID, tg); err != nil {
			t.Fatalf("Upsert(%d) = %v", ruleID, err)
		}
		final[ruleID] = tg // 新增或更新；注意它可能已经在 final 里
	}

	layered, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v, want nil", err)
	}
	expect := buildIndex(t, final)

	// 随机画像逐条比对
	for iter := range 400 {
		p := Profile{}
		for _, f := range fields {
			if rnd.Intn(10) < 3 {
				continue
			}
			p[f] = values[f][:1+rnd.Intn(len(values[f])-1)]
		}
		got := layered.Match(p)
		want := expect.Match(p)
		if !slices.Equal(got, want) {
			t.Fatalf("iter=%d profile=%v\n两层合并 = %v\n单层全量 = %v", iter, p, got, want)
		}
	}

	// 统计也要对得上
	s := layered.Stats()
	if s.BaseRules != len(baseRules) {
		t.Fatalf("Stats().BaseRules = %d, want %d", s.BaseRules, len(baseRules))
	}
	live, deleted := 0, 0
	for _, tg := range db.changed {
		if tg == nil {
			deleted++
		} else {
			live++
		}
	}
	if s.DeltaRules != live || s.Deleted != deleted {
		t.Fatalf("Stats() = %+v, want {DeltaRules:%d Deleted:%d}", s, live, deleted)
	}
}

// TestLayeredConcurrent 变更登记与快照重建可以并发进行，查询侧只读
func TestLayeredConcurrent(t *testing.T) {
	base := buildIndex(t, map[uint64]Targeting{1: {"city": In("bj")}})
	db := NewDeltaBuilder()
	if err := db.Upsert(2, Targeting{"city": In("bj")}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	snap, err := db.Build(base)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	var wg sync.WaitGroup
	// 写入侧：并发登记变更
	for w := range 4 {
		wg.Go(func() {
			for i := range 200 {
				id := uint64(100 + w*1000 + i)
				if i%5 == 0 {
					db.Delete(id)
					continue
				}
				_ = db.Upsert(id, Targeting{"city": In("bj")})
			}
		})
	}
	// 重建侧
	wg.Go(func() {
		for range 50 {
			if _, err := db.Build(base); err != nil {
				t.Errorf("Build() = %v, want nil", err)
				return
			}
		}
	})
	// 查询侧：用固定快照并发查，结果必须稳定
	wg.Go(func() {
		for range 200 {
			if got := snap.Match(Profile{"city": {"bj"}}); !slices.Equal(got, []uint64{1, 2}) {
				t.Errorf("并发查询结果不稳定: %v", got)
				return
			}
		}
	})
	wg.Wait()
}

func TestSkippedRulesErrorMessage(t *testing.T) {
	e := &SkippedRulesError{}
	if e.Error() == "" {
		t.Fatal("空列表也要有可读信息")
	}
	var many []SkippedRule
	for i := range 10 {
		many = append(many, SkippedRule{RuleID: uint64(i), Err: fmt.Errorf("bad")})
	}
	msg := (&SkippedRulesError{Rules: many}).Error()
	if len(msg) > 200 {
		t.Fatalf("规则很多时错误信息不该无限膨胀: %s", msg)
	}
}
