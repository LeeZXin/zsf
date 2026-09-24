package start

import (
	"sync"
	"sync/atomic"
	"testing"
)

// reset 清空已注册钩子，保证各用例互不干扰。
func reset() {
	mu.Lock()
	list = nil
	mu.Unlock()
}

func TestInitOrderStable(t *testing.T) {
	reset()
	var got []string
	AddInit(func() { got = append(got, "b-0") }, 0)
	AddInit(func() { got = append(got, "a-default") })
	AddInit(func() { got = append(got, "c-default") })
	AddInit(func() { got = append(got, "d-2") }, 2)
	for _, fn := range GetInit() {
		fn()
	}
	want := []string{"b-0", "d-2", "a-default", "c-default"}
	// 同 order（默认 10000）的 a、c 必须保持注册顺序（稳定排序）
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

func TestAddInitNilIgnored(t *testing.T) {
	reset()
	AddInit(nil)
	AddInit(nil, 0)
	if got := GetInit(); len(got) != 0 {
		t.Fatalf("nil 钩子应被忽略, got %d", len(got))
	}
}

func TestAddInitConcurrent(t *testing.T) {
	reset()
	var n atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for j := range 50 {
				AddInit(func() { n.Add(1) }, j%3)
			}
		})
	}
	wg.Wait()
	for _, fn := range GetInit() {
		fn()
	}
	if n.Load() != 100 {
		t.Fatalf("并发注册的 100 个钩子应全部执行, got %d", n.Load())
	}
}
