package dynamic

import "sync"

/*
Value 是一个线程安全的泛型值容器，封装了 sync.RWMutex 保护并发访问。
*/
type Value[V any] struct {
	mu sync.RWMutex
	v  V
}

/*
NewValue 创建一个 Value 实例并设置初始值。
通常由 Register 内部使用；业务侧仅在手动构造容器（不接入 Nacos 监听）时直接调用。
*/
func NewValue[V any](v V) *Value[V] {
	return &Value[V]{
		v: v,
	}
}

/*
set 以写锁保护的方式替换内部值。
*/
func (l *Value[V]) set(v V) {
	l.mu.Lock()
	l.v = v
	l.mu.Unlock()
}

/*
Get 以读锁保护的方式返回内部值，可随时读取 Nacos 侧热更新后的最新值。

任意 goroutine 均可并发调用；无阻塞，返回零拷贝的值副本（引用类型为同一底层对象）。
*/
func (l *Value[V]) Get() V {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.v
}
