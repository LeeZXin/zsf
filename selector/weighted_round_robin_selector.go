package selector

import (
	"context"
	"errors"
	"sync"
)

// WeightedRoundRobinSelector 加权平滑路由选择器
type WeightedRoundRobinSelector[T any] struct {
	Nodes       []Node[T]
	selectMutex sync.Mutex
	current     int
	gcd         int
	max         int
}

func (s *WeightedRoundRobinSelector[T]) Select(ctx context.Context, key ...string) (Node[T], error) {
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

func (s *WeightedRoundRobinSelector[T]) maxWeight() int {
	m := 0
	for _, server := range s.Nodes {
		if server.Weight > m {
			m = server.Weight
		}
	}
	return m
}

func (s *WeightedRoundRobinSelector[T]) init() error {
	nodes := s.Nodes
	weights := make([]int, len(nodes))
	for i, node := range nodes {
		if node.Weight <= 0 {
			return errors.New("wrong weight")
		}
		weights[i] = node.Weight
	}
	s.gcd = gcd(weights)
	s.max = max(weights)
	return nil
}

func NewWeightedRoundRobinSelector[T any](nodes []Node[T]) (Selector[T], error) {
	if nodes == nil || len(nodes) == 0 {
		return nil, EmptyNodesErr
	} else if len(nodes) == 1 {
		return &SingleNodeSelector[T]{Node: nodes[0]}, nil
	}
	w := &WeightedRoundRobinSelector[T]{Nodes: nodes}
	err := w.init()
	if err != nil {
		return nil, err
	}
	return w, nil
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
