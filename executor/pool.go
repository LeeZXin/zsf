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
//当超时时间小于等于0时，默认不回收
//queue 队列chan
//workNum 当前协程数量
//rejectHandler 当大于协程池执行能力时的拒绝策略

type Executor struct {
	poolSize       int
	timeout        time.Duration
	queue          chan Runnable
	workNum        int
	rejectStrategy RejectStrategy
	addWorkerMu    sync.Mutex
	cancelFunc     context.CancelFunc
	ctx            context.Context
	closeOnce      sync.Once
	status         int
}

const (
	runningStatus  = 1
	shutdownStatus = 2
)

// NewExecutor 初始化协程池
func NewExecutor(poolSize, queueSize int, timeout time.Duration, rejectStrategy RejectStrategy) (*Executor, error) {
	if poolSize <= 0 {
		return nil, errors.New("pool size should greater than 0")
	}
	if queueSize < 0 {
		return nil, errors.New("queueSize should not less than 0")
	}
	if rejectStrategy == nil {
		return nil, errors.New("nil rejectHandler")
	}
	e := &Executor{
		poolSize:       poolSize,
		timeout:        timeout,
		queue:          make(chan Runnable, queueSize),
		workNum:        0,
		rejectStrategy: rejectStrategy,
		status:         runningStatus,
	}
	e.ctx, e.cancelFunc = context.WithCancel(context.Background())
	return e, nil
}

// ExecuteRunnable 执行任务
// 当前执行任务未达到上限时，新开协程执行
// 否则放入队列
func (e *Executor) ExecuteRunnable(runnable Runnable) error {
	if runnable == nil {
		return errors.New("nil runnable")
	}
	e.addWorkerMu.Lock()
	if e.status == shutdownStatus {
		e.addWorkerMu.Unlock()
		return errors.New("executor is down")
	}
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
	return e.rejectStrategy(runnable, e)
}

// Execute 异步无返回值的执行
func (e *Executor) Execute(fn func()) error {
	if fn == nil {
		return errors.New("nil function")
	}
	return e.ExecuteRunnable(RunnableImpl(fn))
}

// Submit 异步可返回函数执行结果
// 结合promise，可控制返回结果或异常 而非函数本身结果
// 详细看 FutureTask
func (e *Executor) Submit(callable Callable) (*FutureTask, error) {
	if callable == nil {
		return nil, errors.New("nil callable")
	}
	task := NewFutureTask(callable)
	if err := e.ExecuteRunnable(task); err != nil {
		return nil, err
	}
	return task, nil
}

// Shutdown 关闭协程池
func (e *Executor) Shutdown() {
	e.closeOnce.Do(func() {
		e.addWorkerMu.Lock()
		e.status = shutdownStatus
		e.addWorkerMu.Unlock()
		close(e.queue)
		e.cancelFunc()
	})
}

// addWorker 新增协程 并不断监听队列内容
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
