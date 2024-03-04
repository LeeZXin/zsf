package lb

import (
	"context"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/rpcheader"
	"sync"
)

const (
	DefaultVersion = "default"
)

type VersionLoadBalancer struct {
	smu        sync.RWMutex
	allServers []Server
	serversMap map[string]LoadBalancer
	LbPolicy   Policy
}

func (v *VersionLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
	v.smu.Lock()
	defer v.smu.Unlock()
	v.allServers = servers
	v.serversMap = make(map[string]LoadBalancer, 8)
	vServersMap := make(map[string][]Server, 8)
	vServersMap[DefaultVersion] = make([]Server, 0)
	for _, server := range servers {
		version := server.Version
		if version == "" {
			version = DefaultVersion
		}
		vServers, b := vServersMap[version]
		if !b {
			vServers = make([]Server, 0)
		}
		vServers = append(vServers, server)
		vServersMap[version] = vServers
		if version != DefaultVersion {
			vServersMap[DefaultVersion] = append(vServersMap[DefaultVersion], server)
		}
	}
	for version, vServers := range vServersMap {
		balancer := v.getLoadBalancer()
		balancer.SetServers(vServers)
		v.serversMap[version] = balancer
	}
}

func (v *VersionLoadBalancer) GetServers() []Server {
	return v.allServers
}

func (v *VersionLoadBalancer) ChooseServer(ctx context.Context) (Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	version := rpcheader.GetHeaders(ctx).Get(rpcheader.ApiVersion)
	if version == "" {
		version = env.GetVersion()
	}
	v.smu.RLock()
	defer v.smu.RUnlock()
	if len(v.serversMap) == 0 {
		return Server{}, ServerNotFound
	}
	balancer, b := v.serversMap[version]
	if !b {
		balancer = v.serversMap[DefaultVersion]
	}
	ret, err := balancer.ChooseServer(ctx)
	if err != nil {
		return v.serversMap[DefaultVersion].ChooseServer(ctx)
	}
	return ret, nil
}

func (v *VersionLoadBalancer) getLoadBalancer() LoadBalancer {
	switch v.LbPolicy {
	case RoundRobin:
		return new(RoundRobinLoadBalancer)
	case WeightRoundRobin:
		return new(WeightRoundRobinLoadBalancer)
	default:
		return new(RoundRobinLoadBalancer)
	}
}
