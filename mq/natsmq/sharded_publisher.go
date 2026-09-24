package natsmq

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/nats-io/nats.go"
)

type ShardedPublisher struct {
	publishers []*Publisher
}

/*
NewShardedPublisher 连接全部节点并返回发布者。任一节点启动时不可达都会
报错返回（不阻塞等待，已建立的连接会关闭），由外层（如 K8s）重启重试。
AutoCreateSubjectConfig 非空时会在每个节点确保 stream 存在
（不存在则自动创建，已存在不修改）。

典型用法（配合框架 shutdown hook 释放连接）:

	p, err := natsmq.NewShardedPublisher(
		natsmq.PublisherConfig{Addr: addr1},
		natsmq.PublisherConfig{Addr: addr2},
	)
	if err != nil { ... }
	quit.AddHighPriorityShutdownHook(p.Close)
*/
func NewShardedPublisher(cfgList ...PublisherConfig) (*ShardedPublisher, error) {
	if len(cfgList) == 0 {
		return nil, errors.New("natsmq: Publisher Config List are empty")
	}
	publishers := make([]*Publisher, 0, len(cfgList))
	for _, cfg := range cfgList {
		publisher, err := NewPublisher(cfg)
		if err != nil {
			for _, pub := range publishers {
				pub.Close()
			}
			return nil, err
		}
		publishers = append(publishers, publisher)
	}
	return &ShardedPublisher{
		publishers: publishers,
	}, nil
}

/*
Publish 同步发布一条消息到指定 subject，等待 server 确认后返回。
返回 nil 表示消息已持久化到某个节点的 stream。

节点选择：过滤出已连接节点，随机选一个起始偏移、依次尝试，
第一个成功即返回；全部节点失败返回最后一个错误。

固定行为与 Publisher.Publish 一致（NoResponders 重试 2 次、间隔 250ms；
Stream 非空时 ExpectStream 校验；ctx 无 deadline 时 SDK 默认 5s 超时）。
*/
func (p *ShardedPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	return p.publishOnShards(func(publisher *Publisher) error {
		return publisher.Publish(ctx, subject, data)
	})
}

/*
PublishMsg 同步发布带 header 的消息，节点选择与固定行为同 Publish。
*/
func (p *ShardedPublisher) PublishMsg(ctx context.Context, msg *nats.Msg) error {
	return p.publishOnShards(func(publisher *Publisher) error {
		return publisher.PublishMsg(ctx, msg)
	})
}

/*
publishOnShards 过滤可用（已连接）节点，随机起始偏移后依次尝试发布，
第一个成功即返回。无可用节点或全部失败时返回错误。
*/
func (p *ShardedPublisher) publishOnShards(fn func(*Publisher) error) error {
	available := make([]int, 0, len(p.publishers))
	for i, n := range p.publishers {
		if n.conn.IsConnected() {
			available = append(available, i)
		}
	}
	if len(available) == 0 {
		return errors.New("natsmq: no available nats publisher")
	}
	var lastErr error
	for _, i := range shardOrder(available) {
		if err := fn(p.publishers[i]); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("natsmq: publish failed on %d node(s), last error: %w", len(available), lastErr)
}

// shardOrder 返回可用节点的尝试顺序：单节点原样返回；多节点随机起点轮转。
func shardOrder(available []int) []int {
	n := len(available)
	if n <= 1 {
		return available
	}
	start := rand.IntN(n)
	out := make([]int, n)
	for k := range n {
		out[k] = available[(start+k)%n]
	}
	return out
}

/*
Close 关闭全部节点连接，幂等可重复调用。
不使用时调用一次即可（或注册 shutdown hook，见 NewShardedPublisher）。
*/
func (p *ShardedPublisher) Close() {
	for _, publisher := range p.publishers {
		publisher.Close()
	}
}
