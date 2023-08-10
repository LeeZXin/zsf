package zsf

import (
	"github.com/LeeZXin/zsf/executor"
	"sync"
	"time"
)

var (
	lifeCycles     = make([]LifeCycle, 0)
	mu             = sync.Mutex{}
	notifyExecutor *executor.Executor
)

func init() {
	notifyExecutor, _ = executor.NewExecutor(8, 1024, time.Minute, executor.CallerRunsStrategy)
}

type Context struct {
}

type LifeCycle interface {
	// OnApplicationStart 服务启动
	OnApplicationStart()
	// OnApplicationShutdown 服务关闭
	OnApplicationShutdown()
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
	mu.Lock()
	listeners := lifeCycles[:]
	mu.Unlock()
	if len(listeners) == 0 {
		return
	}
	_ = notifyExecutor.Execute(func() {
		for _, listener := range listeners {
			listener.OnApplicationStart()
		}
	})
}

func onApplicationShutdown() {
	mu.Lock()
	listeners := lifeCycles[:]
	mu.Unlock()
	if len(listeners) == 0 {
		return
	}
	_ = notifyExecutor.Execute(func() {
		for _, listener := range listeners {
			listener.OnApplicationShutdown()
		}
	})
}
