// Package pubsub 提供基于 topic 的进程内发布/订阅队列。
//
// 每个订阅者拥有独立的缓冲队列和消费 goroutine，回调按注册顺序依次执行；
// 回调 panic 会被捕获并记录日志，不影响订阅者后续消息的处理。
// 发布为非阻塞语义：订阅者缓冲队列已满时消息被丢弃，发布方永不阻塞。
package pubsub

import (
	"sync"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/utils/idutil"
	"github.com/LeeZXin/zsf/utils/threadutil"
)

// Callback 订阅者处理消息的回调函数，参数为单次 Publish 发布的全部消息。
type Callback func(any)

// Queue 发布/订阅队列接口，按 topic 组织订阅者。
type Queue interface {
	// Publish 向 topic 的所有订阅者发布消息。
	Publish(topic string, messages ...any)
	// Subscribe 订阅 topic 并返回订阅句柄，回调在独立的消费 goroutine 中执行。
	Subscribe(topic string, callbacks ...Callback) *Subscriber
	// Unsubscribe 取消订阅。
	Unsubscribe(*Subscriber)
}

// Subscriber 订阅句柄，由 Subscribe 返回，用于 Unsubscribe 取消订阅。
type Subscriber struct {
	ID        string     // 订阅者唯一标识
	Topic     string     // 订阅的主题
	callbacks []Callback // 回调列表，按注册顺序依次执行
	queue     chan []any // 消息缓冲队列，容量由 New 的 size 指定
}

// queue Queue 的默认实现，订阅表结构为 map[topic]map[subscriberID]*Subscriber。
type queue struct {
	queueSizePerTopic int
	sync.RWMutex
	subscribers map[string]map[string]*Subscriber
}

// Publish 向 topic 的所有订阅者投递消息。
//
// 采用非阻塞发送：订阅者缓冲队列已满时直接丢弃该消息，保证发布方永不阻塞。
// 发送在读锁持有期间进行，与 Unsubscribe 的写锁互斥，因此不会出现向已关闭通道发送的 panic。
func (q *queue) Publish(topic string, messages ...any) {
	q.RLock()
	defer q.RUnlock()
	if subs, ok := q.subscribers[topic]; ok {
		for _, sub := range subs {
			select {
			case sub.queue <- messages:
			default:
			}
		}
	}
}

// Subscribe 为 topic 注册一个订阅者并返回其句柄。
//
// 订阅者拥有独立的缓冲队列和消费 goroutine，回调按注册顺序依次执行；
// 回调 panic 由 threadutil.RunSafe 捕获并记录日志，订阅者保持存活继续处理后续消息。
func (q *queue) Subscribe(topic string, callbacks ...Callback) *Subscriber {
	q.Lock()
	defer q.Unlock()
	sub := &Subscriber{
		ID:        idutil.RandomUUID(),
		Topic:     topic,
		callbacks: callbacks,
		queue:     make(chan []any, q.queueSizePerTopic),
	}
	go func() {
		for message := range sub.queue {
			for _, callback := range sub.callbacks {
				err := threadutil.RunSafe(func() {
					callback(message)
				})
				if err != nil {
					logger.Logger.Error().Err(err).Msgf("subscriber: topic %s id %s callback panic with err: %v", sub.Topic, sub.ID, err)
				}
			}
		}
	}()
	if q.subscribers[topic] == nil {
		q.subscribers[topic] = make(map[string]*Subscriber)
	}
	q.subscribers[topic][sub.ID] = sub
	return sub
}

// Unsubscribe 取消订阅：关闭对应订阅者的消息通道并将其从订阅表移除，重复调用为无害空操作。
//
// 通道只在写锁下关闭，而 Publish 的发送在读锁下进行，且订阅者移除后对 Publish 不可见，
// 因此关闭通道与发送消息不会并发发生——请勿在重构时破坏这一锁纪律。
func (q *queue) Unsubscribe(sub *Subscriber) {
	q.Lock()
	defer q.Unlock()
	if subMap, ok := q.subscribers[sub.Topic]; ok {
		s, b := subMap[sub.ID]
		if b {
			close(s.queue)
			delete(subMap, sub.ID)
		}
		if len(subMap) == 0 {
			delete(q.subscribers, sub.Topic)
		}
	}
}

// New 创建发布/订阅队列，size 指定每个订阅者缓冲队列的容量，queueSizePerTopic <= 0 时默认 100。
func New(queueSizePerTopic int) Queue {
	if queueSizePerTopic <= 0 {
		queueSizePerTopic = 100
	}
	return &queue{
		queueSizePerTopic: queueSizePerTopic,
		subscribers:       make(map[string]map[string]*Subscriber),
	}
}
