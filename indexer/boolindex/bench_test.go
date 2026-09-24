package boolindex

import (
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"slices"
	"testing"
	"time"
)

// README 里「性能与容量」一节的数字全部由这里的压测产生。复现命令：
//
//	go test -run '^$' -bench . -benchtime=200x ./indexer/boolindex/ -benchrules=1000000
//
// 默认只跑 10 万条规则；100 万条那档会占约 800MB 中间内存，内存小的机器别开。
// 自定义 flag 必须放在包路径之后，否则 go test 会把它当成包名。
//
// 数据是**合成的**，不是线上实测：形状对齐广告定向的常见形态，但不是任何真实业务数据。
// 字段数、每条规则的条件数、取值基数、命中率，任何一个变了数字都会明显不同，
// 所以这些数字只当量级参考，要拍板请拿你们自己的数据形态压一遍。

var (
	benchRules    = flag.Int("benchrules", 100000, "压测规则数")
	benchDeltaPct = flag.Int("benchdelta", 5, "分层压测：百分之多少的规则当作「变化过」放进增量层")
)

// benchFields 是合成数据的字段构成：前 6 个是枚举维度，age 是数值范围维度
var benchFields = []struct {
	name string
	card int
}{
	{"city", 300}, {"interest", 5000}, {"device", 20},
	{"gender", 3}, {"app", 2000}, {"ageseg", 50},
}

// genBenchPacks 生成合成规则：每条 2~3 个条件、20% 额外挂一条排除条件、
// 30% 挂一个年龄区间（withRange 时）。命中率由条件数和取值基数共同决定，约 0.2%。
func genBenchPacks(nRules int, withRange bool) []Targeting {
	rnd := rand.New(rand.NewSource(42))
	pick := func(name string, card, n int) []string {
		vs := make([]string, 0, n)
		for range n {
			vs = append(vs, fmt.Sprintf("%s_v%d", name, rnd.Intn(card)))
		}
		return vs
	}

	packs := make([]Targeting, 0, nRules)
	for range nRules {
		perm := rnd.Perm(len(benchFields))
		k := 2 + rnd.Intn(2)
		pack := Targeting{}
		for j := range k {
			f := benchFields[perm[j]]
			pack[Field(f.name)] = In(pick(f.name, f.card, 1+rnd.Intn(5))...)
		}
		if k < len(benchFields) && rnd.Intn(5) == 0 {
			f := benchFields[perm[k]]
			pack[Field(f.name)] = pack[Field(f.name)].WithNotIn(pick(f.name, f.card, 1)...)
		}
		if withRange && rnd.Intn(10) < 3 {
			lo := int64(18 + rnd.Intn(40))
			pack["age"] = Between(lo, lo+int64(rnd.Intn(20)))
		}
		packs = append(packs, pack)
	}
	return packs
}

func genBenchQueries(nQueries int, withRange bool) []Profile {
	rnd := rand.New(rand.NewSource(20240926))
	pick := func(name string, card, n int) []string {
		vs := make([]string, 0, n)
		for range n {
			vs = append(vs, fmt.Sprintf("%s_v%d", name, rnd.Intn(card)))
		}
		return vs
	}

	queries := make([]Profile, 0, nQueries)
	for range nQueries {
		p := Profile{}
		for _, f := range benchFields {
			if rnd.Intn(2) == 0 {
				continue
			}
			p[Field(f.name)] = pick(f.name, f.card, 1+rnd.Intn(2))
		}
		if withRange && rnd.Intn(10) < 7 {
			p["age"] = []string{fmt.Sprintf("%d", 18+rnd.Intn(55))}
		}
		queries = append(queries, p)
	}
	return queries
}

// buildBenchIndex 只计时 AddRule + Build，不含规则生成
func buildBenchIndex(tb testing.TB, packs []Targeting) (*Indexer, time.Duration) {
	tb.Helper()
	tn := time.Now()
	ix := NewIndexer()
	for i, pack := range packs {
		if err := ix.AddRule(uint64(i+1), pack); err != nil {
			tb.Fatalf("AddRule(%d) = %v", i+1, err)
		}
	}
	if err := ix.Build(); err != nil {
		tb.Fatalf("Build() = %v", err)
	}
	return ix, time.Since(tn)
}

// reportIndex 报告索引规模。调用前要保证传入的规则切片已不再被引用，
// 否则测到的是输入数据加索引。
func reportIndex(tb testing.TB, ix *Indexer, buildDur time.Duration) {
	tb.Helper()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s := ix.Stats()
	tb.Logf("规则=%d 倒排记录=%d 建索引=%.2fs 索引堆占用=%.1fMB (%.0f 字节/条)",
		s.Rules, s.Records, buildDur.Seconds(),
		float64(ms.HeapAlloc)/1024/1024, float64(ms.HeapAlloc)/float64(s.Rules))
}

// benchLatency 跑 b.N 次并按分位数报告。Go benchmark 只给均值，
// 而尾延迟才是投放链路真正关心的（长尾来自画像字段多、命中多的查询）。
func benchLatency(b *testing.B, run func(i int)) {
	b.Helper()
	lat := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		run(i)
		lat = append(lat, time.Since(t0))
	}
	b.StopTimer()
	slices.Sort(lat)
	pct := func(p float64) time.Duration { return lat[int(float64(len(lat)-1)*p)] }
	b.Logf("延迟分位 p50=%v p99=%v max=%v", pct(0.50), pct(0.99), lat[len(lat)-1])
}

func benchMatch(tb *testing.B, withRange bool) {
	packs := genBenchPacks(*benchRules, withRange)
	ix, buildDur := buildBenchIndex(tb, packs)
	packs = nil // 让 reportIndex 测到的只有索引
	reportIndex(tb, ix, buildDur)

	queries := genBenchQueries(1000, withRange)
	hits := 0
	for _, q := range queries {
		hits += len(ix.Match(q))
	}
	tb.Logf("平均命中=%.1f 条/查询 (%.4f%%)",
		float64(hits)/float64(len(queries)),
		float64(hits)*100/float64(len(queries))/float64(*benchRules))

	benchLatency(tb, func(i int) {
		_ = ix.Match(queries[i%len(queries)])
	})
}

// benchLayered 建全量层 + 增量层，返回两层快照与增量层重建耗时
func benchLayered(tb testing.TB) (*LayeredIndex, time.Duration) {
	tb.Helper()
	packs := genBenchPacks(*benchRules, false)
	base, baseDur := buildBenchIndex(tb, packs)

	step := 100 / *benchDeltaPct
	db := NewDeltaBuilder()
	changed := 0
	for i, pack := range packs {
		if i%step != 0 {
			continue
		}
		if err := db.Upsert(uint64(i+1), pack); err != nil {
			tb.Fatalf("Upsert(%d) = %v", i+1, err)
		}
		changed++
	}
	tn := time.Now()
	layered, err := db.Build(base)
	deltaDur := time.Since(tn)
	if err != nil {
		tb.Fatalf("Build() = %v", err)
	}
	tb.Logf("全量 %d 条(%.2fs) + 增量 %d 条(%.3fs) -> %+v",
		*benchRules, baseDur.Seconds(), changed, deltaDur.Seconds(), layered.Stats())
	packs = nil // 让后面测到的只有两层索引本身
	reportIndex(tb, base, baseDur)
	return layered, deltaDur
}

// BenchmarkLayeredMatch 两层合并的查询开销（README 分层表格的 avg/p50 那一列）
func BenchmarkLayeredMatch(b *testing.B) {
	layered, _ := benchLayered(b)
	queries := genBenchQueries(1000, false)
	benchLatency(b, func(i int) {
		_ = layered.Match(queries[i%len(queries)])
	})
}

// BenchmarkDeltaBuild 增量层重建耗时（README 分层表格的「增量重建」那一列）
func BenchmarkDeltaBuild(b *testing.B) {
	packs := genBenchPacks(*benchRules, false)
	base, _ := buildBenchIndex(b, packs)
	step := 100 / *benchDeltaPct
	db := NewDeltaBuilder()
	for i, pack := range packs {
		if i%step == 0 {
			if err := db.Upsert(uint64(i+1), pack); err != nil {
				b.Fatalf("Upsert(%d) = %v", i+1, err)
			}
		}
	}
	packs = nil
	runtime.GC()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.Build(base); err != nil {
			b.Fatalf("Build() = %v", err)
		}
	}
}

func BenchmarkMatch(b *testing.B)          { benchMatch(b, false) }
func BenchmarkMatchWithRange(b *testing.B) { benchMatch(b, true) }

// BenchmarkBuild 只测全量重建耗时（README 里「重建 0.9 秒」那个数字）
func BenchmarkBuild(b *testing.B) {
	packs := genBenchPacks(*benchRules, false)
	runtime.GC()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildBenchIndex(b, packs)
	}
}
