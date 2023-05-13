package selector

import (
	"errors"
	"math/rand"
	"sync/atomic"
)

// RoundRobinSelector 轮询路由选择器
type RoundRobinSelector struct {
	Nodes []*Node
	index uint64
	init  bool
}

func (s *RoundRobinSelector) Init() error {
	if s.Nodes == nil || len(s.Nodes) == 0 {
		return errors.New("empty nodes")
	}
	s.index = uint64(rand.Intn(len(s.Nodes)))
	s.init = true
	return nil
}

func (s *RoundRobinSelector) Select(key ...string) (*Node, error) {
	if !s.init {
		return nil, errors.New("call this after init")
	}
	index := atomic.AddUint64(&s.index, 1)
	return s.Nodes[index%uint64(len(s.Nodes))], nil
}
