package discovery

import (
	"context"
	"errors"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/lb"
	"math/rand"
)

var (
	defaultDiscovery Discovery
)

func init() {
	switch static.GetString("discovery.type") {
	case "static":
		defaultDiscovery = NewStaticDiscovery()
	case "etcd":
		defaultDiscovery = NewEtcdDiscovery()
	case "multiEtcd":
		defaultDiscovery = NewMultiEtcdDiscovery()
	}
}

func GetDefaultDiscovery() Discovery {
	return defaultDiscovery
}

func SetDefaultDiscovery(discovery Discovery) {
	defaultDiscovery = discovery
}

type Discovery interface {
	Discover(context.Context, string) ([]lb.Server, error)
	DiscoverWithZone(context.Context, string, string) ([]lb.Server, error)
	ChooseServer(context.Context, string) (lb.Server, error)
	ChooseServerWithZone(context.Context, string, string) (lb.Server, error)
}

func compareServers(s1, s2 []lb.Server) bool {
	if len(s1) != len(s2) {
		return false
	}
	for _, x := range s1 {
		find := false
		for _, y := range s2 {
			if x.IsSameAs(y) {
				find = true
				break
			}
		}
		if !find {
			return false
		}
	}
	return true
}

func Discover(ctx context.Context, name string) ([]lb.Server, error) {
	if defaultDiscovery == nil {
		return nil, errors.New("nil discovery")
	}
	return defaultDiscovery.Discover(ctx, name)
}

func DiscoverWithZone(ctx context.Context, zone string, name string) ([]lb.Server, error) {
	if defaultDiscovery == nil {
		return nil, errors.New("nil discovery")
	}
	return defaultDiscovery.DiscoverWithZone(ctx, zone, name)
}

func ChooseServer(ctx context.Context, name string) (lb.Server, error) {
	if defaultDiscovery == nil {
		return lb.Server{}, errors.New("nil discovery")
	}
	return defaultDiscovery.ChooseServer(ctx, name)
}

func ChooseServerWithZone(ctx context.Context, zone string, name string) (lb.Server, error) {
	if defaultDiscovery == nil {
		return lb.Server{}, errors.New("nil discovery")
	}
	return defaultDiscovery.ChooseServerWithZone(ctx, zone, name)
}

func ChooseRandomServer(servers []lb.Server) lb.Server {
	if len(servers) == 0 {
		return lb.Server{}
	}
	if len(servers) == 1 {
		return servers[0]
	}
	return servers[rand.Int()%len(servers)]
}
