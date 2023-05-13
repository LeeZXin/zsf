package selector

import (
	"math/rand"
	"sync/atomic"
)

// RoundRobinSelector 轮询路由选择器
type RoundRobinSelector struct {
	Nodes []Node
	index uint64
}

func (s *RoundRobinSelector) init() {
	s.index = uint64(rand.Intn(len(s.Nodes)))
}

func (s *RoundRobinSelector) Select(key ...string) (Node, error) {
	index := atomic.AddUint64(&s.index, 1)
	return s.Nodes[index%uint64(len(s.Nodes))], nil
}

func NewRoundRobinSelector(nodes []Node) (Selector, error) {
	if nodes == nil || len(nodes) == 0 {
		return nil, EmptyNodesErr
	} else if len(nodes) == 1 {
		return &SingleNodeSelector{Node: nodes[0]}, nil
	}
	r := &RoundRobinSelector{Nodes: nodes}
	r.init()
	return r, nil
}
