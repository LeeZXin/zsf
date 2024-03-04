package discovery

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/lb"
	"github.com/spf13/cast"
)

var (
	discoveryType = static.GetString("discovery.type")
	lbPolicy      = static.GetString("discovery.lbPolicy")

	multiEtcd     = make(map[string]Discovery)
	discoveryImpl Discovery
	localZone     string
)

const (
	StaticDiscoveryType    = "static"
	EtcdDiscoveryType      = "etcd"
	MultiEtcdDiscoveryType = "multiEtcd"
)

func init() {
	switch discoveryType {
	case StaticDiscoveryType:
		discoveryImpl = newStaticDiscovery()
	case EtcdDiscoveryType:
		discoveryImpl = newEtcdDiscovery(
			static.GetString("discovery.etcd.endpoints"),
			static.GetString("discovery.etcd.username"),
			static.GetString("discovery.etcd.password"),
		)
	case MultiEtcdDiscoveryType:
		localZone = static.GetString("discovery.zone")
		if localZone == "" {
			logger.Logger.Fatalf("empty discovery.zone")
		}
		etcdCfgs := static.GetMapSlice("discovery.multi")
		for _, etcdCfg := range etcdCfgs {
			zone := cast.ToString(etcdCfg["zone"])
			if zone == "" {
				continue
			}
			_, b := multiEtcd[zone]
			if b {
				logger.Logger.Fatalf("duplicated discovery.multi.zone")
			}
			endpoints := cast.ToString(etcdCfg["endpoints"])
			username := cast.ToString(etcdCfg["username"])
			password := cast.ToString(etcdCfg["password"])
			multiEtcd[zone] = newEtcdDiscovery(endpoints, username, password)
		}
	}
}

type Discovery interface {
	Discover(context.Context, string) ([]lb.Server, error)
	ChooseServer(context.Context, string) (lb.Server, error)
}

func ChooseServer(ctx context.Context, name string) (lb.Server, error) {
	if discoveryType == MultiEtcdDiscoveryType {
		return ChooseServerWithZone(ctx, localZone, name)
	}
	return discoveryImpl.ChooseServer(ctx, name)
}

func Discover(ctx context.Context, name string) ([]lb.Server, error) {
	if discoveryImpl == nil {
		return nil, lb.ServerNotFound
	}
	return discoveryImpl.Discover(ctx, name)
}

func DiscoverWithZone(ctx context.Context, zone, name string) ([]lb.Server, error) {
	discovery, b := multiEtcd[zone]
	if !b {
		return nil, lb.ServerNotFound
	}
	return discovery.Discover(ctx, name)
}

func ChooseServerWithZone(ctx context.Context, zone, name string) (lb.Server, error) {
	discovery, b := multiEtcd[zone]
	if !b {
		return lb.Server{}, lb.ServerNotFound
	}
	return discovery.ChooseServer(ctx, name)
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
