/*
Package chunktask 提供批量数据处理器，按数量或时间间隔触发批量回调。

典型使用场景：

	曝光日志、点击日志等高频写入场景，不适合逐条写数据库。
	将数据积攒到一定数量（如 100 条）或达到时间间隔（如 5 秒）后，
	批量写入数据库，减少 DB 交互次数。

设计特点：
  - 双重触发：达到 max 数量 → 立即回调；达到 duration 时间 → 定时回调
  - 优雅关闭：高优先级 shutdown hook 确保进程退出前剩余数据被刷出
  - 线程安全：所有操作通过 sync.Mutex 保护

使用示例:

	task := chunktask.New[ClickLog](100, 5*time.Second, func(batch []ClickLog) {
	    // 批量写入数据库
	    repo.BatchInsert(batch)
	})

	task.Add(clickLog1, clickLog2, clickLog3)  // 分散在不同请求中调用

注意：
  - 达到阈值或定时触发时先释放锁再执行 callback，callback 内可再次 Add（不会自死锁）；
    并发 Add 可能交错产生多批回调，callback 仍须线程安全
  - callback 不宜耗时过长，否则积压只表现为延迟刷出，不再堵住 Add
  - 交给 callback 的切片随后不再被 Task 使用，callback 可持有引用
*/
package chunktask

import (
	"sync"
	"time"

	"github.com/LeeZXin/zsf/quit"
)

/*
Task 批量处理器，泛型参数 T 为业务数据类型。
*/
type Task[T any] struct {
	mu       sync.Mutex
	data     []T
	max      int
	duration time.Duration
	callback func([]T)
}

/*
tick 启动定时刷新和关闭钩子。

1. 注册高优先级 shutdown hook：进程退出前将剩余数据刷出
2. 启动定时器 goroutine：每隔 duration 时间检查并刷出积攒的数据

定时器在 quit.Stopping 关闭后退出；shutdown hook 仍会再 Sync 一次，避免残留。
*/
func (t *Task[T]) tick() {
	quit.AddHighPriorityShutdownHook(t.Sync)
	go func() {
		ticker := time.NewTicker(t.duration)
		defer ticker.Stop()
		for {
			select {
			case <-quit.Stopping():
				return
			case <-ticker.C:
				t.Sync()
			}
		}
	}()
}

/*
Sync 立即将缓冲区积攒的数据刷给 callback（不等待数量阈值或定时器），
通常由 shutdown hook 调用，保证进程退出前数据不丢失。
刷出后缓冲区与 tick/Add 触发时一致地被重置，可重复调用（无数据时为空操作），
不会重复消费。
*/
func (t *Task[T]) takeLocked() []T {
	if len(t.data) == 0 {
		return nil
	}
	batch := t.data
	t.data = make([]T, 0, t.max)
	return batch
}

func (t *Task[T]) flush(batch []T) {
	if len(batch) > 0 {
		t.callback(batch)
	}
}

/*
Sync 立即将缓冲区积攒的数据刷给 callback（不等待数量阈值或定时器），
通常由 shutdown hook 调用，保证进程退出前数据不丢失。
刷出后缓冲区与 tick/Add 触发时一致地被重置，可重复调用（无数据时为空操作），
不会重复消费。
*/
func (t *Task[T]) Sync() {
	t.mu.Lock()
	batch := t.takeLocked()
	t.mu.Unlock()
	t.flush(batch)
}

/*
Add 添加数据到缓冲区。若达到 max 上限则立即触发批量回调。

callback 在锁外执行：内部再调用 Add 不会自死锁；panic 也不会把锁留死。
*/
func (t *Task[T]) Add(data ...T) {
	t.mu.Lock()
	t.data = append(t.data, data...)
	var batch []T
	if len(t.data) >= t.max {
		batch = t.takeLocked()
	}
	t.mu.Unlock()
	t.flush(batch)
}

/*
New 创建批量处理器。

参数：
  - max:      触发批量回调的数据量阈值
  - duration: 定时刷新的时间间隔
  - callback: 批量回调函数，接收当前积攒的数据切片

callback 中的切片由 Task 内部管理，回调返回后不应再引用。
*/
func New[T any](max int, duration time.Duration, callback func([]T)) *Task[T] {
	task := &Task[T]{
		max:      max,
		duration: duration,
		callback: callback,
		data:     make([]T, 0, max),
	}
	task.tick()
	return task
}
