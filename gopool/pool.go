/*
Package gopool 提供带容量上限的协程池，用于复用 worker 协程执行短任务。

实现要点：

  - 任务入队与 worker 领取在同一把锁（taskLock）下完成，提交/领取均线程安全；
    worker 协程与任务对象经 sync.Pool 复用，降低高频提交下的分配开销

  - worker 数量惰性增长：有任务且当前 worker 数小于容量上限时才新建 worker，
    上限即 NewPool / NewPoolWithQueue 的 worker 容量

  - 等待队列有长度上限：NewPool 默认为 workerCap*256；队列满时 Go/CtxGo
    静默丢弃（与 Close 之后提交相同，不阻塞调用方）。queueCap<=0 表示不限制

  - Close 为阻塞语义：置 closed 后等待所有 worker 退出（已入队任务会被消费
    完毕）才返回；创建时已注册高优先级 shutdown hook，在 HTTP/gRPC 停听后再调用

坑（务必注意）：
  - Close 之后或等待队列已满时，再 Go/CtxGo 提交的任务会被静默丢弃（不执行也不报错）
  - 任务函数内 panic 会被 worker 捕获并记日志，该任务结束、worker 继续取下一个，
    避免把整个进程打死或让 Close 永久阻塞；仍建议提交方自行保证任务不 panic
  - CtxGo 提交的任务在 ctx 已取消/超时时会被 worker 跳过（不执行）；
    任务执行过程本身不随 ctx 中断，需要协作式取消请在任务函数内自行检查 ctx
*/
package gopool

import (
	"context"
	"sync"

	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
)

// defaultQueuePerWorker NewPool 每个 worker 对应的默认等待队列额度。
const defaultQueuePerWorker int32 = 256

// Pool 协程池接口，Go/CtxGo 提交任务（并发安全），Close 阻塞等待所有 worker
// 退出。任务按入队顺序被 worker 领取执行（多 worker 并发，完成顺序不定）。
type Pool interface {
	Go(f func())
	CtxGo(ctx context.Context, f func())
	Close()
}

var taskPool sync.Pool

func init() {
	start.AddInit(func() {
		taskPool.New = newTask
	}, -7)
}

type task struct {
	ctx  context.Context
	f    func()
	next *task
}

func (t *task) zero() {
	t.ctx = nil
	t.f = nil
	t.next = nil
}

func (t *task) Recycle() {
	t.zero()
	taskPool.Put(t)
}

func newTask() any {
	return &task{}
}

type pool struct {
	cap         int32
	queueCap    int32 // 等待队列上限；<=0 表示不限制
	taskCount   int32 // 链表中尚未被领取的任务数
	taskHead    *task
	taskTail    *task
	taskLock    sync.Mutex
	workerCount int32
	closed      bool
	closeWait   chan struct{}
	closeOnce   sync.Once // 保证 closeWait 只被 close 一次，Close 可重复调用
}

// NewPool 创建最大 worker 数为 cap 的协程池，等待队列上限为 cap*256。
// cap < 1 时按 1 处理。创建时自动注册高优先级 shutdown hook。
func NewPool(cap int32) Pool {
	if cap < 1 {
		cap = 1
	}
	return NewPoolWithQueue(cap, cap*defaultQueuePerWorker)
}

// NewPoolWithQueue 创建协程池：workerCap 为最大 worker 数，queueCap 为等待队列上限。
// workerCap < 1 时按 1 处理；queueCap <= 0 表示等待队列不限制长度。
func NewPoolWithQueue(workerCap, queueCap int32) Pool {
	if workerCap < 1 {
		workerCap = 1
	}
	p := &pool{
		cap:       workerCap,
		queueCap:  queueCap,
		closeWait: make(chan struct{}),
	}
	quit.AddHighPriorityShutdownHook(p.Close)
	return p
}

// Go 提交任务到协程池执行，等价于 CtxGo(context.Background(), f)。
func (p *pool) Go(f func()) {
	p.CtxGo(context.Background(), f)
}

// CtxGo 提交任务到协程池执行。
// ctx 用于任务是否执行的裁决：worker 领取任务时若 ctx 已取消/超时，
// 该任务被跳过（不执行）。任务执行过程本身不随 ctx 中断，
// 需要协作式取消请在任务函数内部自行检查 ctx。
func (p *pool) CtxGo(ctx context.Context, f func()) {
	p.taskLock.Lock()
	defer p.taskLock.Unlock()
	if p.closed {
		return
	}
	if p.queueCap > 0 && p.taskCount >= p.queueCap {
		return
	}
	t := taskPool.Get().(*task)
	t.ctx = ctx
	t.f = f
	if p.taskHead == nil {
		p.taskHead = t
		p.taskTail = t
	} else {
		p.taskTail.next = t
		p.taskTail = t
	}
	p.taskCount++
	if p.workerCount == 0 || p.workerCount < p.cap {
		p.workerCount++
		w := workerPool.Get().(*worker)
		w.pool = p
		w.run()
	}
}

// Close 关闭协程池：不再接受新任务（此后提交被静默丢弃），
// 等待所有 worker 退出后返回；已入队任务仍会被现有 worker 消费完毕。
// 幂等：可重复调用（shutdown hook 与业务方都调用也安全）。
func (p *pool) Close() {
	p.taskLock.Lock()
	p.closed = true
	if p.workerCount == 0 {
		p.closeOnce.Do(func() { close(p.closeWait) })
	}
	p.taskLock.Unlock()
	<-p.closeWait
}

func (p *pool) decWorkerCountWithoutLock() {
	p.workerCount--
	if p.closed && p.workerCount <= 0 {
		p.closeOnce.Do(func() { close(p.closeWait) })
	}
}

func (p *pool) popTaskLocked() *task {
	if p.taskHead == nil {
		return nil
	}
	t := p.taskHead
	p.taskHead = p.taskHead.next
	if p.taskHead == nil {
		p.taskTail = nil
	}
	p.taskCount--
	t.next = nil
	return t
}
