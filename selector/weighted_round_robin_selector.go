package selector

import (
	"errors"
	"sync"
)

// WeightedRoundRobinSelector 加权平滑路由选择器
type WeightedRoundRobinSelector struct {
	Nodes       []*Node
	selectMutex sync.Mutex
	current     int
	gcd         int
	max         int
	init        bool
}

func (s *WeightedRoundRobinSelector) Select(key ...string) (*Node, error) {
	if !s.init {
		return nil, errors.New("call this after init")
	}
	s.selectMutex.Lock()
	defer s.selectMutex.Unlock()
	for {
		s.current = (s.current + 1) % len(s.Nodes)
		if s.current == 0 {
			s.max -= s.gcd
			if s.max <= 0 {
				s.max = s.maxWeight()
			}
		}
		if s.Nodes[s.current].Weight >= s.max {
			return s.Nodes[s.current], nil
		}
	}
}

func (s *WeightedRoundRobinSelector) maxWeight() int {
	m := 0
	for _, server := range s.Nodes {
		if server.Weight > m {
			m = server.Weight
		}
	}
	return m
}

func (s *WeightedRoundRobinSelector) Init() error {
	nodes := s.Nodes
	if nodes == nil || len(nodes) == 0 {
		return errors.New("empty nodes")
	}
	weights := make([]int, len(nodes))
	for i, node := range nodes {
		if node.Weight <= 0 {
			return errors.New("wrong weight")
		}
		weights[i] = node.Weight
	}
	s.gcd = gcd(weights)
	s.max = max(weights)
	s.init = true
	return nil
}
