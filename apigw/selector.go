package apigw

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/services/discovery"
	"sync"
	"sync/atomic"
)

type hostSelector interface {
	Select(context.Context) (string, error)
}

type ipPortSelector struct {
	serviceName string
}

func (s *ipPortSelector) Select(ctx context.Context) (string, error) {
	server, err := discovery.ChooseServer(ctx, s.serviceName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", server.Host, server.Port), nil
}

type nilSelector struct{}

func (s *nilSelector) Select(context.Context) (string, error) {
	return "", nil
}

type emptyTargetsSelector struct{}

func (s *emptyTargetsSelector) Select(context.Context) (string, error) {
	return "", errors.New("empty targets")
}

type roundRobinSelector struct {
	targets []string
	index   *atomic.Uint64
}

func newRoundRobinSelector(targets []string) hostSelector {
	if len(targets) == 0 {
		return new(emptyTargetsSelector)
	}
	index := atomic.Uint64{}
	index.Store(0)
	return &roundRobinSelector{
		targets: targets,
		index:   &index,
	}
}

func (s *roundRobinSelector) Select(context.Context) (string, error) {
	lenTargets := len(s.targets)
	if lenTargets == 0 {
		return "", errors.New("empty targets")
	}
	return s.targets[s.index.Add(1)%uint64(lenTargets)], nil
}

type weightedTarget struct {
	weight int
	target string
}

type weightedRoundRobinSelector struct {
	targets   []weightedTarget
	maxWeight int
	current   int
	gcd       int
	max       int
	sync.Mutex
}

func newWeightedRoundRobinSelector(targets []weightedTarget) hostSelector {
	if len(targets) == 0 {
		return new(emptyTargetsSelector)
	}
	w := new(weightedRoundRobinSelector)
	w.targets = targets
	weights := make([]int, len(w.targets))
	for i := range w.targets {
		weights[i] = w.targets[i].weight
	}
	w.maxWeight = weights[0]
	for i := 1; i < len(weights); i++ {
		weight := weights[i]
		if weight > w.maxWeight {
			w.maxWeight = weight
		}
	}
	w.gcd = gcd(weights)
	w.max = max(weights)
	return w
}

func (w *weightedRoundRobinSelector) Select(context.Context) (string, error) {
	w.Lock()
	defer w.Unlock()
	lenTargets := len(w.targets)
	if lenTargets == 0 {
		return "", errors.New("empty targets")
	}
	for {
		w.current = (w.current + 1) % len(w.targets)
		if w.current == 0 {
			w.max -= w.gcd
			if w.max <= 0 {
				w.max = w.maxWeight
			}
		}
		if w.targets[w.current].weight >= w.max {
			return w.targets[w.current].target, nil
		}
	}
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
