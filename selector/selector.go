package selector

import (
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
	NewSelectorFuncMap = map[string]func([]Node) (Selector, error){
		WeightedRoundRobinPolicy: NewWeightedRoundRobinSelector,
		RoundRobinPolicy:         NewRoundRobinSelector,
		HashPolicy:               NewHashSelector,
	}

	EmptyNodesErr = errors.New("empty nodes")
)

// Selector 路由选择器interface
type Selector interface {
	// Select 选择
	Select(key ...string) (Node, error)
}

// Node 路由节点信息
type Node struct {
	Id     string `json:"id"`
	Data   any    `json:"data"`
	Weight int    `json:"weight"`
}
