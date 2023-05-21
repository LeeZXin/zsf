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

type Runnable interface {
	Run()
}

type RunnableImpl struct {
	Runnable func()
}

func (r *RunnableImpl) Run() {
	if r.Runnable != nil {
		r.Runnable()
	}
}

type futureResult struct {
	Result any
	Err    error
}

type Callable func() (any, error)

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

func (t *FutureTask) completed() {
	t.doneOnce.Do(func() {
		close(t.done)
	})
}

func (t *FutureTask) setObj(result futureResult) bool {
	return t.result.CompareAndSwap(nil, result)
}

func (t *FutureTask) SetResult(result any) bool {
	if t.setObj(futureResult{
		Result: result,
	}) {
		defer t.completed()
		return true
	}
	return false
}

func (t *FutureTask) SetError(err error) bool {
	if t.setObj(futureResult{
		Err: err,
	}) {
		defer t.completed()
		return true
	}
	return false
}

func (t *FutureTask) Get() (any, error) {
	return t.GetWithTimeout(0)
}

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
		val := t.result.Load()
	}
	res, ok := val.(futureResult)
	return res.Result, res.Err 
}
