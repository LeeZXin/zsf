package psub

import (
	"errors"
	"github.com/LeeZXin/zsf-utils/executor"
	"runtime"
	"sync"
	"time"
)

//本地事件广播
//常用于不同模块之间的通信 解耦领域
//不同领域之间通信

//有默认的channel实现
//也可新增一个channel

//使用map和协程池实现

var (
	nilErr          = errors.New("nil data")
	noSubscriberErr = errors.New("no subscriber")
	invalidArgErr   = errors.New("invalid arguments")

	defaultChannel *Channel[any]
)

type Channel[T any] struct {
	mu       sync.RWMutex
	ch       map[string][]Subscriber[T]
	executor *executor.Executor
}

type Subscriber[T any] func(data T)

// Publish 发布数据
func (c *Channel[T]) Publish(topic string, data T) error {
	if topic == "" {
		return nilErr
	}
	c.mu.RLock()
	subs, ok := c.ch[topic]
	if ok {
		subs = subs[:]
	}
	c.mu.RUnlock()
	if !ok {
		return noSubscriberErr
	}
	return c.executor.Execute(func() {
		for _, sub := range subs {
			sub(data)
		}
	})
}

// Subscribe 订阅数据
func (c *Channel[T]) Subscribe(topic string, subscriber Subscriber[T]) error {
	if topic == "" || subscriber == nil {
		return invalidArgErr
	}
	c.mu.Lock()
	subs, ok := c.ch[topic]
	if !ok {
		subs = make([]Subscriber[T], 0)
	}
	c.ch[topic] = append(subs, subscriber)
	c.mu.Unlock()
	return nil
}

func (c *Channel[T]) Shutdown() {
	if c.executor != nil {
		c.executor.Shutdown()
	}
}

// NewChannel 初始化channel 入参是协程池
func NewChannel[T any](executor *executor.Executor) (*Channel[T], error) {
	if executor == nil {
		return nil, invalidArgErr
	}
	return &Channel[T]{
		mu:       sync.RWMutex{},
		ch:       make(map[string][]Subscriber[T]),
		executor: executor,
	}, nil
}

func init() {
	// 默认带实现队列长度为1024的协程池
	e, _ := executor.NewExecutor(runtime.GOMAXPROCS(0), 1024, 10*time.Minute, executor.CallerRunsStrategy)
	channel, _ := NewChannel[any](e)
	defaultChannel = channel
}

func Publish(topic string, data any) error {
	return defaultChannel.Publish(topic, data)
}

func Subscribe(topic string, subscriber Subscriber[any]) error {
	return defaultChannel.Subscribe(topic, subscriber)
}
