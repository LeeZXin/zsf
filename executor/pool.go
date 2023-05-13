package executor

import (
	"context"
	"errors"
	"sync"
	"time"
)

// 协程池封装
// 与java线程池类似, 但没有java线程池的corePoolSize，觉得没必要
// 有Execute(Runnable)
// 和Submit(Callable)
// 当执行任务大于poolSize时，会将任务放入队列等待，否则新加协程执行
// 当queueSize = 0 时，相当于java的synchronousQueue

//poolSize 协程数量大小
//timeout 协程超时时间，当协程空闲到达timeout，会回收协程
//queue 队列chan
//workNum 当前协程数量
//rejectHandler 当大于协程池执行能力时的拒绝策略

type Executor struct {
	poolSize      int
	timeout       time.Duration
	queue         chan Runnable
	workNum       int
	rejectHandler RejectHandler
	addWorkerMu   sync.Mutex
	cancelFunc    context.CancelFunc
	ctx           context.Context
	closeOnce     sync.Once
}

func NewExecutor(poolSize, queueSize int, timeout time.Duration, rejectHandler RejectHandler) (*Executor, error) {
	if poolSize <= 0 {
		return nil, errors.New("pool size should greater than 0")
	}
	if queueSize < 0 {
		return nil, errors.New("queueSize should not less than 0")
	}
	if rejectHandler == nil {
		return nil, errors.New("nil rejectHandler")
	}
	e := &Executor{
		poolSize:      poolSize,
		timeout:       timeout,
		queue:         make(chan Runnable, queueSize),
		workNum:       0,
		rejectHandler: rejectHandler,
	}
	e.ctx, e.cancelFunc = context.WithCancel(context.Background())
	return e, nil
}

func (e *Executor) Execute(runnable Runnable) error {
	if runnable == nil {
		return errors.New("nil runnable")
	}
	e.addWorkerMu.Lock()
	if e.workNum < e.poolSize && e.addWorker(runnable) {
		e.workNum += 1
		e.addWorkerMu.Unlock()
		return nil
	}
	e.addWorkerMu.Unlock()
	select {
	case e.queue <- runnable:
		return nil
	default:
		break
	}
	return e.rejectHandler.RejectedExecution(runnable, e)
}

func (e *Executor) Submit(callable Callable) (*FutureTask, error) {
	if callable == nil {
		return nil, errors.New("nil callable")
	}
	task := NewFutureTask(callable)
	if err := e.Execute(task); err != nil {
		return nil, err
	}
	return task, nil
}

func (e *Executor) Shutdown() {
	e.closeOnce.Do(func() {
		close(e.queue)
		e.cancelFunc()
	})
}

func (e *Executor) addWorker(runnable Runnable) bool {
	w := worker{
		timeout:       e.timeout,
		queue:         e.queue,
		ctx:           e.ctx,
		firstRunnable: runnable,
		onClose: func(w *worker) {
			e.addWorkerMu.Lock()
			defer e.addWorkerMu.Unlock()
			e.workNum -= 1
		},
	}
	w.Run()
	return true
}
