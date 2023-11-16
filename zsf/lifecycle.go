package zsf

import (
	"sync"
)

var (
	lifeCycles = make([]LifeCycle, 0)
	mu         = sync.Mutex{}
)

type LifeCycle interface {
	// OnApplicationStart 服务启动
	OnApplicationStart()
	// AfterInitialize 启动后
	AfterInitialize()
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
	lc := lifeCycles[:]
	mu.Unlock()
	for _, l := range lc {
		l.OnApplicationStart()
	}
}

func onApplicationShutdown() {
	mu.Lock()
	lc := lifeCycles[:]
	mu.Unlock()
	for _, l := range lc {
		l.OnApplicationShutdown()
	}
}

func afterInitialize() {
	mu.Lock()
	lc := lifeCycles[:]
	mu.Unlock()
	for _, l := range lc {
		l.AfterInitialize()
	}
}
