package lb

import (
	"context"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/rpcheader"
)

const (
	DefaultVersion = "default"
)

type versionLoadBalancer struct {
	allServers []Server
	serversMap map[string]LoadBalancer
	LbPolicy   Policy
}

func (v *versionLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
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

func (v *versionLoadBalancer) GetServers() []Server {
	return v.allServers
}

func (v *versionLoadBalancer) ChooseServer(ctx context.Context) (Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	version := rpcheader.GetHeaders(ctx).Get(rpcheader.ApiVersion)
	if version == "" {
		version = env.GetVersion()
	}
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

func (v *versionLoadBalancer) getLoadBalancer() LoadBalancer {
	switch v.LbPolicy {
	case RoundRobin:
		return new(roundRobinLoadBalancer)
	case WeightRoundRobin:
		return new(weightRoundRobinLoadBalancer)
	default:
		return new(roundRobinLoadBalancer)
	}
}
