package natsmq

import (
	"slices"
	"testing"
)

func TestShardOrderSingleNodeKeepsIndex(t *testing.T) {
	got := shardOrder([]int{2})
	if !slices.Equal(got, []int{2}) {
		t.Fatalf("单节点应使用 available 下标，got %v", got)
	}
}

func TestShardOrderEmpty(t *testing.T) {
	if got := shardOrder(nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestShardOrderPermutation(t *testing.T) {
	in := []int{0, 3, 7}
	got := shardOrder(in)
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	seen := map[int]int{}
	for _, v := range got {
		seen[v]++
	}
	for _, v := range in {
		if seen[v] != 1 {
			t.Fatalf("不是原集合的排列：in=%v got=%v", in, got)
		}
	}
}
