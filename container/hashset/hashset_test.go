package hashset

import "testing"

func TestClear(t *testing.T) {
	s := NewHashset(1, 2, 3)
	s.Clear()
	if s.Len() != 0 {
		t.Fatalf("len=%d", s.Len())
	}
	s.Add(4)
	if !s.Contains(4) {
		t.Fatal("Clear 后应仍可 Add")
	}
}
