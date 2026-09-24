package hashmap

import "testing"

func TestSwapReturnsPrevious(t *testing.T) {
	m := NewSyncMap[string, int]()
	prev, loaded := m.Swap("k", 1)
	if loaded || prev != 0 {
		t.Fatalf("空 map Swap 应为 (zero,false), got %d %v", prev, loaded)
	}
	prev, loaded = m.Swap("k", 2)
	if !loaded || prev != 1 {
		t.Fatalf("覆盖 Swap 应返回旧值, got %d %v", prev, loaded)
	}
	got, ok := m.Load("k")
	if !ok || got != 2 {
		t.Fatalf("Load=%d %v", got, ok)
	}
}
