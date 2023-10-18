package balancer

import (
	"github.com/LeeZXin/zsf-utils/selector"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

// newRrBuilder creates a new round-robin balancer builder.
func newRrBuilder() balancer.Builder {
	return base.NewBalancerBuilder(
		selector.RoundRobinPolicy,
		&pickerBuilder{
			lbPolicy: selector.RoundRobinPolicy,
		},
		base.Config{
			HealthCheck: true,
		},
	)
}

func init() {
	balancer.Register(newRrBuilder())
}
