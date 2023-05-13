package balancer

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"zsf/selector"
)

// newRrBuilder creates a new weighted-round-robin balancer builder.
func newRrBuilder() balancer.Builder {
	return base.NewBalancerBuilder(string(selector.RoundRobinPolicy), &pickerBuilder{lbPolicy: selector.RoundRobinPolicy}, base.Config{HealthCheck: true})
}

func init() {
	balancer.Register(newRrBuilder())
}
