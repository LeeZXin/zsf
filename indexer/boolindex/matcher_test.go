package boolindex

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"
)

// bruteMatch 按文档语义直接求值，作为差分测试的基准实现。
// 刻意写得与索引实现无关（纯集合比较），这样两者同时错的概率很低。
func bruteMatch(packs []Targeting, p Profile) bool {
	for _, pack := range packs {
		if brutePack(pack, p) {
			return true
		}
	}
	return false
}

func brutePack(pack Targeting, p Profile) bool {
	for f, cond := range pack {
		pv := p[f]
		if cond.hasRange() {
			if !bruteRangeMatch(cond, pv) {
				return false
			}
			continue
		}
		// 暴力求值刻意只走公开访问器，不碰内部字段：
		// 这样它同时也在检验访问器暴露的信息够不够外部判断一条规则
		in, notIn, _ := cond.Enum()
		if len(in) > 0 && !intersect(pv, in) {
			return false
		}
		if len(notIn) > 0 && intersect(pv, notIn) {
			return false
		}
	}
	return true
}

func intersect(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

type randomData struct {
	index    *Indexer
	packs    map[uint64][]Targeting
	fields   []Field
	values   map[Field][]string
	numField Field
}

func genRandomData(rnd *rand.Rand, ruleCnt int) *randomData {
	d := &randomData{
		packs:  map[uint64][]Targeting{},
		values: map[Field][]string{},
	}
	// 字段基数刻意取小（3~5 个取值），保证随机查询能真的命中一批规则
	for i := range 4 {
		f := Field(fmt.Sprintf("f%d", i))
		d.fields = append(d.fields, f)
		vals := make([]string, 0, 5)
		for v := range 3 + rnd.Intn(3) {
			vals = append(vals, fmt.Sprintf("%s_v%d", f, v))
		}
		d.values[f] = vals
	}
	// 再加一个数值范围字段：取值域取小，让区间大量重叠。
	// 它不进 d.fields —— 那个列表只装枚举字段。
	d.numField = "num"
	for v := range 21 {
		d.values[d.numField] = append(d.values[d.numField], fmt.Sprintf("%d", v))
	}

	d.index = NewIndexer()
	for i := 1; i <= ruleCnt; i++ {
		id := uint64(i)
		packCnt := 1 + rnd.Intn(3)
		packs := make([]Targeting, 0, packCnt)
		for range packCnt {
			pack := Targeting{}
			for _, f := range d.fields {
				r := rnd.Intn(10)
				switch {
				case r < 4: // 40% 正向条件
					pack[f] = In(d.pick(rnd, f, 1+rnd.Intn(2))...)
				case r < 5: // 10% 双向条件
					pack[f] = In(d.pick(rnd, f, 1)...).WithNotIn(d.pick(rnd, f, 1)...)
				case r < 7: // 20% 只看排除
					pack[f] = NotIn(d.pick(rnd, f, 1)...)
				}
			}
			// 数值字段独立决定：30% 加一条随机区间
			if rnd.Intn(10) < 3 {
				lo := rnd.Intn(20)
				hi := lo + rnd.Intn(10)
				switch rnd.Intn(4) {
				case 0:
					pack[d.numField] = Between(int64(lo), int64(hi))
				case 1:
					pack[d.numField] = GreaterThan(int64(lo))
				case 2:
					pack[d.numField] = LessThan(int64(hi))
				default:
					pack[d.numField] = AtLeast(int64(lo))
				}
			}
			if len(pack) == 0 {
				pack[d.fields[rnd.Intn(len(d.fields))]] = In(d.pick(rnd, d.fields[0], 1)...)
			}
			packs = append(packs, pack)
		}
		if err := d.index.AddRule(id, packs...); err != nil {
			panic(fmt.Sprintf("AddRule(%d) = %v", id, err))
		}
		d.packs[id] = packs
	}
	if err := d.index.Build(); err != nil {
		panic(fmt.Sprintf("Build() = %v", err))
	}
	return d
}

func (d *randomData) pick(rnd *rand.Rand, f Field, n int) []string {
	vals := d.values[f]
	out := make([]string, 0, n)
	for range n {
		out = append(out, vals[rnd.Intn(len(vals))])
	}
	return out
}

func (d *randomData) genProfile(rnd *rand.Rand) Profile {
	p := Profile{}
	// 数值字段：60% 给 1~2 个整数取值，偶尔给个非法值
	if rnd.Intn(10) < 6 {
		n := 1 + rnd.Intn(2)
		vals := make([]string, 0, n)
		for range n {
			vals = append(vals, fmt.Sprintf("%d", rnd.Intn(25)-2))
		}
		if rnd.Intn(10) == 0 {
			vals = append(vals, "not_a_number")
		}
		p[d.numField] = vals
	}
	for _, f := range d.fields {
		if rnd.Intn(10) < 3 { // 30% 画像缺该字段
			continue
		}
		n := 1 + rnd.Intn(3)
		vals := make([]string, 0, n)
		for range n {
			vals = append(vals, d.pick(rnd, f, 1)[0])
		}
		if rnd.Intn(10) == 0 { // 偶尔塞入索引里不存在的取值
			vals = append(vals, "unknown_value")
		}
		p[f] = vals
	}
	if len(p) == 0 && rnd.Intn(2) == 0 {
		return nil // 偶尔生成 nil 画像
	}
	return p
}

// TestDifferential 随机规则 + 随机画像，与暴力枚举逐条对拍。
func TestDifferential(t *testing.T) {
	rnd := rand.New(rand.NewSource(20240924))
	d := genRandomData(rnd, 300)

	for iter := range 500 {
		p := d.genProfile(rnd)

		want := make([]uint64, 0, 16)
		for id, packs := range d.packs {
			if bruteMatch(packs, p) {
				want = append(want, id)
			}
		}
		slices.Sort(want)

		got := d.index.Match(p)
		if !slices.Equal(got, want) {
			t.Fatalf("iter=%d profile=%v\n got=%v\nwant=%v", iter, p, got, want)
		}
	}
}

// TestConcurrentMatch Build 之后索引只读，多 goroutine 并发查询结果必须一致。
func TestConcurrentMatch(t *testing.T) {
	rnd := rand.New(rand.NewSource(11))
	d := genRandomData(rnd, 500)

	profiles := make([]Profile, 0, 64)
	wants := make([][]uint64, 0, 64)
	for range 64 {
		p := d.genProfile(rnd)
		profiles = append(profiles, p)
		wants = append(wants, d.index.Match(p))
	}

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for i := range 300 {
				idx := i % len(profiles)
				got := d.index.Match(profiles[idx])
				if !slices.Equal(got, wants[idx]) {
					t.Errorf("并发结果不一致: profile=%v\n got=%v\nwant=%v", profiles[idx], got, wants[idx])
					return
				}
			}
		})
	}
	wg.Wait()
}
