package gopool

import (
	"sync"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/utils/threadutil"
)

var workerPool sync.Pool

func init() {
	start.AddInit(func() {
		workerPool.New = newWorker
	}, -7)
}

type worker struct {
	pool *pool
}

func newWorker() any {
	return &worker{}
}

func (w *worker) run() {
	go func() {
		for {
			w.pool.taskLock.Lock()
			t := w.pool.popTaskLocked()
			if t == nil {
				w.close()
				w.pool.taskLock.Unlock()
				w.Recycle()
				return
			}
			w.pool.taskLock.Unlock()
			// 任务 ctx 已取消/超时则跳过执行（提交方已不再关心结果）
			if t.ctx != nil && t.ctx.Err() != nil {
				t.Recycle()
				continue
			}
			if err := threadutil.RunSafe(t.f); err != nil {
				logger.Logger.Error().Err(err).Msg("gopool: task panic")
			}
			t.Recycle()
		}
	}()
}

func (w *worker) close() {
	w.pool.decWorkerCountWithoutLock()
}

func (w *worker) zero() {
	w.pool = nil
}

func (w *worker) Recycle() {
	w.zero()
	workerPool.Put(w)
}
