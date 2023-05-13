package selector

import (
	"errors"
)

//负载均衡策略选择器通用封装
//用于rpc的节点负载均衡或其他负载均衡实现

// LbPolicy 负载均衡策略
// 目前只实现轮询、加权平滑轮询、哈希
type LbPolicy string

const (
	RoundRobinPolicy         = LbPolicy("round_robin")
	WeightedRoundRobinPolicy = LbPolicy("weighted_round_robin")
	HashPolicy               = LbPolicy("hash_policy")
)

var (
	NewSelectorFuncMap = map[LbPolicy]func([]*Node) Selector{
		WeightedRoundRobinPolicy: func(nodes []*Node) Selector {
			if nodes == nil || len(nodes) == 0 {
				return &ErrorSelector{Err: errors.New("empty nodes")}
			} else if len(nodes) == 1 {
				return &SingleNodeSelector{Node: nodes[0]}
			}
			return &WeightedRoundRobinSelector{Nodes: nodes}
		},
		RoundRobinPolicy: func(nodes []*Node) Selector {
			if nodes == nil || len(nodes) == 0 {
				return &ErrorSelector{Err: errors.New("empty nodes")}
			} else if len(nodes) == 1 {
				return &SingleNodeSelector{Node: nodes[0]}
			}
			return &RoundRobinSelector{Nodes: nodes}
		},
		HashPolicy: func(nodes []*Node) Selector {
			if nodes == nil || len(nodes) == 0 {
				return &ErrorSelector{Err: errors.New("empty nodes")}
			} else if len(nodes) == 1 {
				return &SingleNodeSelector{Node: nodes[0]}
			}
			return &HashSelector{Nodes: nodes}
		},
	}
)

// Selector 路由选择器interface
type Selector interface {
	// Init 初始化
	Init() error
	// Select 选择
	Select(key ...string) (*Node, error)
}

// Node 路由节点信息
type Node struct {
	Id     string `json:"id"`
	Data   any    `json:"data"`
	Weight int    `json:"weight"`
}

func gcd(numbers []int) int {
	result := numbers[0]
	for _, number := range numbers[1:] {
		result = gcdTwoNumbers(result, number)
	}
	return result
}

func gcdTwoNumbers(a, b int) int {
	for b != 0 {
		t := b
		b = a % b
		a = t
	}
	return a
}

func max(numbers []int) int {
	m := numbers[0]
	for _, number := range numbers[1:] {
		if number > m {
			m = number
		}
	}
	return m
}
