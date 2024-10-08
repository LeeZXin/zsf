package lb

import (
	"context"
)

type weightRoundRobinLoadBalancer struct {
	allServers []Server
	maxWeight  int
	current    int
	gcd        int
	max        int
}

func (w *weightRoundRobinLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
	w.allServers = servers
	weights := make([]int, len(w.allServers))
	for i := range w.allServers {
		weights[i] = w.allServers[i].Weight
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
}

func (w *weightRoundRobinLoadBalancer) GetServers() []Server {
	return w.allServers
}

func (w *weightRoundRobinLoadBalancer) ChooseServer(_ context.Context) (Server, error) {
	if len(w.allServers) == 0 {
		return Server{}, ServerNotFound
	}
	for {
		w.current = (w.current + 1) % len(w.allServers)
		if w.current == 0 {
			w.max -= w.gcd
			if w.max <= 0 {
				w.max = w.maxWeight
			}
		}
		if w.allServers[w.current].Weight >= w.max {
			return w.allServers[w.current], nil
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
