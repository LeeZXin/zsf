package quit

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestShutdownTimeoutDefault(t *testing.T) {
	if ShutdownTimeout(0) != DefaultShutdownTimeout {
		t.Fatalf("0 应回退默认超时")
	}
	if ShutdownTimeout(-1) != DefaultShutdownTimeout {
		t.Fatalf("负数应回退默认超时")
	}
	if ShutdownTimeout(5*time.Second) != 5*time.Second {
		t.Fatalf("正数应原样返回")
	}
}

func TestHookRunOrder(t *testing.T) {
	var got []string
	AddApplicationShutdownHook(func() { got = append(got, "app") })
	AddHighPriorityShutdownHook(func() { got = append(got, "high") })
	AddLowPriorityShutdownHook(func() { got = append(got, "low") })
	AddFinalShutdownHook(func() { got = append(got, "final") })
	app, high, low, final := snapshotHooks()
	runHooks(app)
	runHooks(high)
	runHooks(low)
	runHooks(final)
	want := []string{"app", "high", "low", "final"}
	if len(got) < len(want) {
		t.Fatalf("钩子未按四级执行: %v", got)
	}
	// 只校验本用例追加的四段相对顺序（可能夹杂其它测试注册的钩子）
	idx := map[string]int{}
	for i, s := range got {
		if _, ok := idx[s]; !ok {
			idx[s] = i
		}
	}
	if !(idx["app"] < idx["high"] && idx["high"] < idx["low"] && idx["low"] < idx["final"]) {
		t.Fatalf("顺序应为 app < high < low < final, got %v", got)
	}
}

func TestAddHookNilIgnored(t *testing.T) {
	AddApplicationShutdownHook(nil)
	AddHighPriorityShutdownHook(nil)
	AddLowPriorityShutdownHook(nil)
	AddFinalShutdownHook(nil)
}

func TestAddHookConcurrent(t *testing.T) {
	var n atomic.Int32
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			AddHighPriorityShutdownHook(func() { n.Add(1) })
		}
		close(done)
	}()
	for i := 0; i < 50; i++ {
		AddHighPriorityShutdownHook(func() { n.Add(1) })
	}
	<-done
}
