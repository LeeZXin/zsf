package cacheutil

import "sync"

/*
Locked 是一个线程安全的泛型值容器，封装了 sync.RWMutex 保护并发访问。
*/
type Locked[V any] struct {
	mu sync.RWMutex
	v  V
}

/*
NewLocked 创建一个 Locked 实例并设置初始值。
*/
func NewLocked[V any](v V) *Locked[V] {
	return &Locked[V]{
		v: v,
	}
}

/*
Set 以写锁保护的方式替换内部值。
*/
func (l *Locked[V]) Set(v V) {
	l.mu.Lock()
	l.v = v
	l.mu.Unlock()
}

/*
Get 以读锁保护的方式返回内部值。
*/
func (l *Locked[V]) Get() V {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.v
}

/*
WithLock 在写锁保护下执行 fn，保证读-改-写的原子性。

注意：fn 执行期间持有写锁，应尽快完成操作，避免在 fn 内做 IO 或长时间阻塞。
fn 若 panic 说明调用方代码存在 bug，通用组件不做 recover 兜底。
*/
func (l *Locked[V]) WithLock(fn func(V)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(l.v)
}
