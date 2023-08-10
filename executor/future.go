package executor

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

//协程池封装
//类java runnable和future
//这个future是promiseFuture
//可以修改任意返回值的future
//利用atomic.Value和chan轻松实现

var (
	TimeoutError = errors.New("task timeout")
)

// Runnable 任务执行接口
type Runnable interface {
	Run()
}

// RunnableImpl 默认实现类
type RunnableImpl func()

func (r RunnableImpl) Run() {
	r()
}

// futureResult promise result
// cas控制返回结果
type futureResult struct {
	Result any
	Err    error
}

// Callable 带返回值的任务
type Callable func() (any, error)

// FutureTask 与java类似
type FutureTask struct {
	result   atomic.Value
	callable Callable
	done     chan struct{}
	doneOnce sync.Once
}

func NewFutureTask(callable Callable) *FutureTask {
	return &FutureTask{
		result:   atomic.Value{},
		callable: callable,
		done:     make(chan struct{}),
	}
}

func (t *FutureTask) Run() {
	res, err := t.callable()
	t.setObj(futureResult{
		Result: res,
		Err:    err,
	})
	t.completed()
}

// completed 通知完成
func (t *FutureTask) completed() {
	t.doneOnce.Do(func() {
		close(t.done)
	})
}

// setObj cas结果
func (t *FutureTask) setObj(result futureResult) bool {
	return t.result.CompareAndSwap(nil, result)
}

// SetResult 执行中可随意控制返回callable返回结果
func (t *FutureTask) SetResult(result any) bool {
	if t.setObj(futureResult{
		Result: result,
	}) {
		defer t.completed()
		return true
	}
	return false
}

// SetError 执行中可随意控制返回callable返回异常
func (t *FutureTask) SetError(err error) bool {
	if t.setObj(futureResult{
		Err: err,
	}) {
		defer t.completed()
		return true
	}
	return false
}

// Get 阻塞获取结果 无限期等待
func (t *FutureTask) Get() (any, error) {
	return t.GetWithTimeout(0)
}

// GetWithTimeout 带超时返回结果 超时返回timeoutErr
func (t *FutureTask) GetWithTimeout(timeout time.Duration) (any, error) {
	val := t.result.Load()
	if val == nil {
		if timeout > 0 {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-t.done:
				break
			case <-timer.C:
				return nil, TimeoutError
			}
		} else {
			select {
			case <-t.done:
				break
			}
		}
		val = t.result.Load()
	}
	res, _ := val.(futureResult)
	return res.Result, res.Err
}
