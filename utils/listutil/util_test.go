package listutil

import (
	"errors"
	"slices"
	"testing"
)

func TestDistinctKeepsFirstAndOrder(t *testing.T) {
	got := Distinct(1, 2, 1, 3, 2, 4)
	if !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("got=%v", got)
	}
}

func TestAllNe(t *testing.T) {
	if !AllNe([]int{2, 4, 6}, func(n int) bool { return n%2 == 0 }) {
		t.Fatal("expected all even")
	}
	if AllNe([]int{2, 3, 4}, func(n int) bool { return n%2 == 0 }) {
		t.Fatal("expected not all even")
	}
	if !AllNe([]int{}, func(int) bool { return false }) {
		t.Fatal("empty should be true")
	}
}

func TestFlatMapNe(t *testing.T) {
	got := FlatMapNe([]int{1, 2, 3}, func(n int) []int {
		return []int{n, n * 10}
	})
	if !slices.Equal(got, []int{1, 10, 2, 20, 3, 30}) {
		t.Fatalf("got=%v", got)
	}
}

func TestDistinctByNeKeepsFirstAndOrder(t *testing.T) {
	type item struct {
		id    int
		value string
	}
	got := DistinctByNe([]item{{1, "a"}, {2, "b"}, {1, "c"}, {3, "d"}, {2, "e"}}, func(it item) int {
		return it.id
	})
	want := []item{{1, "a"}, {2, "b"}, {3, "d"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestDistinctByError(t *testing.T) {
	boom := errors.New("boom")
	got, err := DistinctBy([]int{1, 2, 3}, func(n int) (int, error) {
		if n == 2 {
			return 0, boom
		}
		return n, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("出错时应为 nil, got=%v", got)
	}
}

func TestFlatNePreservesOrder(t *testing.T) {
	got := FlatNe([][]int{{1, 2}, nil, {3}, {}, {4, 5}})
	if !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("got=%v", got)
	}
}

func TestConcatAllDoesNotMutateInputs(t *testing.T) {
	a := []int{1, 2}
	b := []int{3}
	got := ConcatAll(a, b)
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("got=%v", got)
	}
	if !slices.Equal(a, []int{1, 2}) || !slices.Equal(b, []int{3}) {
		t.Fatalf("入参被修改: a=%v b=%v", a, b)
	}
}
