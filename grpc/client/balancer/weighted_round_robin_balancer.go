package balancer

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"zsf/selector"
)

// newWrrBuilder creates a new weighted-round-robin balancer builder.
func newWrrBuilder() balancer.Builder {
	return base.NewBalancerBuilder(string(selector.WeightedRoundRobinPolicy), &pickerBuilder{lbPolicy: selector.WeightedRoundRobinPolicy}, base.Config{HealthCheck: true})
}

func init() {
	balancer.Register(newWrrBuilder())
}
