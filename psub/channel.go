package psub

import (
	"errors"
	"github.com/LeeZXin/zsf/executor"
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

	defaultChannel *Channel
)

type Channel struct {
	mu       sync.RWMutex
	ch       map[string][]Subscriber
	executor *executor.Executor
}

type Subscriber func(data any)

// Publish 发布数据
func (c *Channel) Publish(topic string, data any) error {
	if topic == "" || data == nil {
		return nilErr
	}
	c.mu.RLock()
	subs, ok := c.ch[topic]
	c.mu.RUnlock()
	if !ok {
		return noSubscriberErr
	}
	return c.executor.Execute(func() {
			for _, sub := range subs {
				sub(data)
			}
		}
	})
}

// Subscribe 订阅数据
func (c *Channel) Subscribe(topic string, subscriber Subscriber) error {
	if topic == "" || subscriber == nil {
		return invalidArgErr
	}
	c.mu.Lock()
	subs, ok := c.ch[topic]
	if !ok {
		subs = make([]Subscriber, 0)
	}
	c.ch[topic] = append(subs, subscriber)
	c.mu.Unlock()
	return nil
}

func (c *Channel) Shutdown() {
	if c.executor != nil {
		c.executor.Shutdown()
	}
}

// NewChannel 初始化channel 入参是协程池
func NewChannel(executor *executor.Executor) (*Channel, error) {
	if executor == nil {
		return nil, invalidArgErr
	}
	return &Channel{
		mu:       sync.RWMutex{},
		ch:       make(map[string][]Subscriber),
		executor: executor,
	}, nil
}

func init() {
	// 默认带实现队列长度为1024的协程池
	e, _ := executor.NewExecutor(runtime.GOMAXPROCS(0), 1024, 10*time.Minute, &executor.CallerRunsPolicy{})
	channel, _ := NewChannel(e)
	defaultChannel = channel
}

func Publish(topic string, data any) error {
	return defaultChannel.Publish(topic, data)
}

func Subscribe(topic string, subscriber Subscriber) error {
	return defaultChannel.Subscribe(topic, subscriber)
}
