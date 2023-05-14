package zsf

import (
	"github.com/LeeZXin/zsf/executor"
	"sync"
	"time"
)

var (
	lifeCycles     = make([]LifeCycle, 0)
	mu             = sync.RWMutex{}
	notifyExecutor *executor.Executor
)

func init() {
	notifyExecutor, _ = executor.NewExecutor(8, 1024, time.Minute, &executor.CallerRunsPolicy{})
}

type Context struct {
}

type LifeCycle interface {
	// OnApplicationStart 服务启动
	OnApplicationStart(ctx Context)
	// OnApplicationShutdown 服务关闭
	OnApplicationShutdown(ctx Context)
}

func RegisterApplicationLifeCycle(lifeCycle LifeCycle) {
	if lifeCycle == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	lifeCycles = append(lifeCycles, lifeCycle)
}

func onApplicationStart() {
	c := copyCtx()
	mu.RLock()
	listeners := lifeCycles[:]
	mu.RUnlock()
	if len(listeners) == 0 {
		return
	}
	_ = notifyExecutor.Execute(&executor.RunnableImpl{
		Runnable: func() {
			for _, listener := range listeners {
				listener.OnApplicationStart(c)
			}
		},
	})
}

func onApplicationShutdown() {
	c := copyCtx()
	mu.RLock()
	listeners := lifeCycles[:]
	mu.RUnlock()
	if len(listeners) == 0 {
		return
	}
	_ = notifyExecutor.Execute(&executor.RunnableImpl{
		Runnable: func() {
			for _, listener := range listeners {
				listener.OnApplicationShutdown(c)
			}
		},
	})
}

func copyCtx() Context {
	return Context{}
}
