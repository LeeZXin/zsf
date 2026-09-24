package countlimiter

import (
	"sync"
	"sync/atomic"
)

type Limiter struct {
	mu        sync.Mutex
	container map[string]*atomic.Int64
}

func New() *Limiter {
	return &Limiter{
		container: make(map[string]*atomic.Int64),
	}
}

func (l *Limiter) Acquire(scope string, limit int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	counter, ok := l.container[scope]
	if !ok {
		counter = new(atomic.Int64)
		l.container[scope] = counter
	}
	if counter.Load() >= limit {
		return false
	}
	counter.Add(1)
	return true
}

func (l *Limiter) Release(scope string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	counter, ok := l.container[scope]
	if !ok {
		return
	}
	if counter.Add(-1) <= 0 {
		delete(l.container, scope)
	}
}
