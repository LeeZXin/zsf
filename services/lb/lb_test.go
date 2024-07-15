package lb

import (
	"fmt"
	"github.com/LeeZXin/zsf/rpcheader"
	"testing"
)

func TestRoundRobinLoadBalancer_ChooseServer(t *testing.T) {
	lb := new(roundRobinLoadBalancer)
	lb.SetServers([]Server{
		{
			Name: "1",
		},
		{
			Name: "2",
		},
		{
			Name: "3",
		},
	})
	for i := 0; i < 10; i++ {
		fmt.Println(lb.ChooseServer(nil))
	}
}

func TestWeightedRoundRobinLoadBalancer_ChooseServer(t *testing.T) {
	lb := new(weightRoundRobinLoadBalancer)
	lb.SetServers([]Server{
		{
			Name:   "1",
			Weight: 2,
		},
		{
			Name:   "2",
			Weight: 5,
		},
		{
			Name:   "3",
			Weight: 3,
		},
	})
	for i := 0; i < 10; i++ {
		fmt.Println(lb.ChooseServer(nil))
	}
}

func TestVersionRoundRobinLoadBalancer_ChooseServer(t *testing.T) {
	lb := &versionLoadBalancer{
		LbPolicy: WeightRoundRobin,
	}
	lb.SetServers([]Server{
		{
			Name:    "1",
			Weight:  2,
			Version: "aa",
		},
		{
			Name:    "2",
			Weight:  5,
			Version: "aa",
		},
		{
			Name:    "3",
			Weight:  3,
			Version: "bb",
		},
		{
			Name:    "4",
			Weight:  4,
			Version: "bb",
		},
		{
			Name:    "5",
			Weight:  3,
			Version: "bb",
		},
	})
	ctx := rpcheader.SetHeaders(nil, map[string]string{
		rpcheader.ApiVersion: "cc",
	})
	for i := 0; i < 20; i++ {
		fmt.Println(lb.ChooseServer(ctx))
	}
}

func TestNearbyRoundRobinLoadBalancer_ChooseServer(t *testing.T) {
	lb := &NearbyLoadBalancer{
		LbPolicy: RoundRobin,
	}
	lb.SetServers([]Server{
		{
			Name:    "1",
			Weight:  2,
			Version: "aa",
			Region:  "r1",
			Zone:    "z1",
		},
		{
			Name:    "2",
			Weight:  5,
			Version: "aa",
			Region:  "r1",
			Zone:    "z2",
		},
		{
			Name:    "3",
			Weight:  3,
			Version: "bb",
			Region:  "r2",
			Zone:    "z1",
		},
		{
			Name:    "4",
			Weight:  4,
			Version: "bb",
			Region:  "r2",
			Zone:    "z2",
		},
		{
			Name:    "5",
			Weight:  3,
			Version: "bb",
			Region:  "r2",
			Zone:    "z1",
		},
	})
	ctx := rpcheader.SetHeaders(nil, map[string]string{
		rpcheader.ApiVersion: "cc",
	})
	for i := 0; i < 20; i++ {
		fmt.Println(lb.ChooseServer(ctx))
	}
}
