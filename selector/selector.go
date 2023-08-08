package selector

import (
	"context"
	"errors"
)

//负载均衡策略选择器通用封装
//用于rpc的节点负载均衡或其他负载均衡实现

// lbPolicy 负载均衡策略
// 目前只实现轮询、加权平滑轮询、哈希

const (
	RoundRobinPolicy         = "round_robin"
	WeightedRoundRobinPolicy = "weighted_round_robin"
	HashPolicy               = "hash_policy"
)

var (
	EmptyNodesErr = errors.New("empty nodes")
)

// Selector 路由选择器interface
type Selector[T any] interface {
	// Select 选择
	Select(ctx context.Context, key ...string) (Node[T], error)
}

// Node 路由节点信息
type Node[T any] struct {
	Id     string `json:"id"`
	Data   T      `json:"data"`
	Weight int    `json:"weight"`
}

func FindNewSelectorFunc[T any](lbPolicy string) (func(nodes []Node[T]) (Selector[T], error), bool) {
	switch lbPolicy {
	case RoundRobinPolicy:
		return NewRoundRobinSelector[T], true
	case WeightedRoundRobinPolicy:
		return NewWeightedRoundRobinSelector[T], true
	case HashPolicy:
		return NewHashSelector[T], true
	default:
		return nil, false
	}
}
