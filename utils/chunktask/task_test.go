package chunktask

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAddCallbackDoesNotDeadlock(t *testing.T) {
	var n atomic.Int32
	var task *Task[int]
	task = New(2, time.Hour, func(batch []int) {
		n.Add(int32(len(batch)))
		// 锁外回调：内部再 Add 不应死锁
		task.Add(9)
	})
	done := make(chan struct{})
	go func() {
		task.Add(1, 2)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback 内 Add 疑似死锁")
	}
	if n.Load() < 2 {
		t.Fatalf("首批未回调，n=%d", n.Load())
	}
}
