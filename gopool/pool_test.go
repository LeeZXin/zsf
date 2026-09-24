package gopool

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseAfterTaskPanic(t *testing.T) {
	p := NewPool(1)
	p.Go(func() { panic("task boom") })
	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("任务 panic 后 Close 不应永久阻塞")
	}
}

func TestQueueCapDropsOverflow(t *testing.T) {
	p := NewPoolWithQueue(1, 2)
	block := make(chan struct{})
	started := make(chan struct{})
	var ran atomic.Int32
	p.Go(func() {
		close(started)
		<-block
		ran.Add(1)
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("首个任务未开始")
	}
	p.Go(func() { ran.Add(1) })
	p.Go(func() { ran.Add(1) })
	p.Go(func() { ran.Add(1) }) // 队列已满，应丢弃
	close(block)
	p.Close()
	if n := ran.Load(); n != 3 {
		t.Fatalf("队列满应丢弃超额任务, ran=%d want 3", n)
	}
}

func TestUnboundedQueueWhenCapZero(t *testing.T) {
	p := NewPoolWithQueue(1, 0)
	var ran atomic.Int32
	const n = 32
	block := make(chan struct{})
	started := make(chan struct{})
	p.Go(func() {
		close(started)
		<-block
	})
	<-started
	for range n {
		p.Go(func() { ran.Add(1) })
	}
	close(block)
	p.Close()
	if ran.Load() != n {
		t.Fatalf("queueCap=0 不应丢任务, ran=%d", ran.Load())
	}
}
