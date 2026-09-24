package listutil

import (
	"errors"
	"slices"
	"strconv"
	"testing"
)

func TestPipeFilterThenMapTypeChange(t *testing.T) {
	got, err := NewPipe([]int{1, 2, 3, 4}).
		FilterNe(func(n int) bool { return n%2 == 0 }).
		MapNe(strconv.Itoa).
		Data()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !slices.Equal(got, []string{"2", "4"}) {
		t.Fatalf("got=%v", got)
	}
}

func TestPipeFilterMutatesReceiverNotInput(t *testing.T) {
	in := []int{1, 2, 3, 4}
	p := NewPipe(in)
	even := p.FilterNe(func(n int) bool { return n%2 == 0 })
	if even != p {
		t.Fatal("FilterNe 应返回接收者自身")
	}
	got, err := p.Data()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !slices.Equal(in, []int{1, 2, 3, 4}) {
		t.Fatalf("入参被修改: %v", in)
	}
	if !slices.Equal(got, []int{2, 4}) {
		t.Fatalf("接收者应已被过滤, got=%v", got)
	}
}

func TestPipeFilterErrorShortCircuitsMap(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	got, err := NewPipe([]int{1, 2, 3}).
		Filter(func(n int) (bool, error) {
			if n == 2 {
				return false, boom
			}
			return true, nil
		}).
		MapNe(func(n int) int {
			calls++
			return n
		}).
		Data()
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("出错时应为 nil, got=%v", got)
	}
	if calls != 0 {
		t.Fatalf("出错后不应再 Map, calls=%d", calls)
	}
}

func TestPipeMapErrorShortCircuitsFilter(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	got, err := NewPipe([]int{1, 2, 3}).
		Map(func(n int) (string, error) {
			if n == 2 {
				return "", boom
			}
			return strconv.Itoa(n), nil
		}).
		FilterNe(func(s string) bool {
			calls++
			return s != ""
		}).
		Data()
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("出错时应为 nil, got=%v", got)
	}
	if calls != 0 {
		t.Fatalf("出错后不应再 Filter, calls=%d", calls)
	}
}

func TestPipeErrMatchesData(t *testing.T) {
	boom := errors.New("boom")
	p := NewPipe([]int{1}).Map(func(int) (int, error) {
		return 0, boom
	})
	if !errors.Is(p.Err(), boom) {
		t.Fatalf("Err()=%v", p.Err())
	}
	got, err := p.Data()
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("Data()=(%v, %v)", got, err)
	}
}

func TestPipeDistinctByKeepsFirst(t *testing.T) {
	type item struct {
		id    int
		value string
	}
	in := []item{{1, "a"}, {2, "b"}, {1, "c"}}
	p := NewPipe(in)

	uniq, err := p.DistinctByNe(func(it item) int { return it.id }).Data()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !slices.Equal(uniq, []item{{1, "a"}, {2, "b"}}) {
		t.Fatalf("got=%v", uniq)
	}
	if !slices.Equal(in, []item{{1, "a"}, {2, "b"}, {1, "c"}}) {
		t.Fatalf("入参被修改: %v", in)
	}
}

func TestPipeFrameworkOps(t *testing.T) {
	in := []int{3, 1, 2, 1, 4}
	p := NewPipe(in)

	got, err := p.Concat([]int{5}).SortFunc(func(a, b int) int { return a - b }).Skip(1).Limit(3).Data()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("got=%v", got)
	}
	if !slices.Equal(in, []int{3, 1, 2, 1, 4}) {
		t.Fatalf("入参被修改: %v", in)
	}

	flat, err := NewPipe([]int{1, 2}).FlatMapNe(func(n int) []string {
		return []string{strconv.Itoa(n), strconv.Itoa(n * 10)}
	}).Data()
	if err != nil {
		t.Fatalf("flat err=%v", err)
	}
	if !slices.Equal(flat, []string{"1", "10", "2", "20"}) {
		t.Fatalf("flat=%v", flat)
	}

	sum, err := NewPipe([]int{1, 2, 3}).ReduceNe(0, func(acc, n int) int { return acc + n })
	if err != nil || sum != 6 {
		t.Fatalf("sum=(%v, %v)", sum, err)
	}

	m, err := NewPipe([]string{"a", "b", "a"}).ToMapListNe(func(s string) (string, string) {
		return s, s
	})
	if err != nil || len(m["a"]) != 2 || len(m["b"]) != 1 {
		t.Fatalf("group=%v err=%v", m, err)
	}

	first, ok, err := NewPipe([]int{1, 2, 3}).FilterNe(func(n int) bool { return n > 1 }).FindFirstNe(func(n int) bool { return n%2 == 1 })
	if err != nil || !ok || first != 3 {
		t.Fatalf("find=(%v, %v, %v)", first, ok, err)
	}

	all, err := NewPipe([]int{2, 4}).AllNe(func(n int) bool { return n%2 == 0 })
	if err != nil || !all {
		t.Fatalf("all=(%v, %v)", all, err)
	}
}

func TestPipeTerminalsPropagateErr(t *testing.T) {
	boom := errors.New("boom")
	p := NewPipe([]int{1}).Filter(func(int) (bool, error) { return false, boom })

	if _, err := p.Len(); !errors.Is(err, boom) {
		t.Fatalf("Len err=%v", err)
	}
	if _, _, err := p.First(); !errors.Is(err, boom) {
		t.Fatalf("First err=%v", err)
	}
	if _, err := p.ToMapNe(func(n int) (int, int) { return n, n }); !errors.Is(err, boom) {
		t.Fatalf("ToMapNe err=%v", err)
	}
	if _, err := p.ReduceNe(0, func(acc, n int) int { return acc + n }); !errors.Is(err, boom) {
		t.Fatalf("ReduceNe err=%v", err)
	}
}

func TestPipeDistinctByErrorShortCircuits(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	got, err := NewPipe([]int{1, 2, 3}).
		DistinctBy(func(n int) (int, error) {
			if n == 2 {
				return 0, boom
			}
			return n, nil
		}).
		MapNe(func(n int) int {
			calls++
			return n
		}).
		Data()
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("出错时应为 nil, got=%v", got)
	}
	if calls != 0 {
		t.Fatalf("出错后不应再 Map, calls=%d", calls)
	}
}
