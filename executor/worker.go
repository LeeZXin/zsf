package executor

import (
	"context"
	"time"
)

// 协程包装
// 利用chan获取任务
// worker工作协程

type workerOnClose func(*worker)

type worker struct {
	timeout       time.Duration
	queue         chan Runnable
	ctx           context.Context
	firstRunnable Runnable
	onClose       workerOnClose
}

func (w *worker) Run() {
	go func() {
		if w.firstRunnable != nil {
			w.firstRunnable.Run()
			w.firstRunnable = nil
		}
		for {
			task, b, b2 := w.pollTask(w.timeout)
			if b2 || !b {
				break
			}
			if task != nil {
				task.Run()
			}
		}
		if w.onClose != nil {
			w.onClose(w)
		}
	}()
}

func (w *worker) pollTask(duration time.Duration) (Runnable, bool, bool) {
	var runnable Runnable
	if duration > 0 {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		// 监听任务chan
		// 超时回收信号
		// 协程池关闭chan
		select {
		case runnable = <-w.queue:
			return runnable, true, false
		case <-timer.C:
			return nil, false, false
		case <-w.ctx.Done():
			return nil, false, true
		}
	} else {
		select {
		case runnable = <-w.queue:
			return runnable, true, false
		case <-w.ctx.Done():
			return nil, false, true
		}
	}
}
