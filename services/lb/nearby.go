package lb

import (
	"context"
	"github.com/LeeZXin/zsf/common"
	"sync"
)

type nearbyLb struct {
	alb LoadBalancer
	// region找不到 就从所有服务列表里找
	rlb map[string]regionLb
}

type regionLb struct {
	alb LoadBalancer
	// zone 找不到 就从regionLb找
	zlb map[string]LoadBalancer
}

// NearbyLoadBalancer 多区域就近路由 region->zone
type NearbyLoadBalancer struct {
	allServers []Server
	LbPolicy   Policy
	lb         nearbyLb
	smu        sync.RWMutex
}

func (r *NearbyLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
	r.smu.Lock()
	defer r.smu.Unlock()
	r.allServers = servers
	r.lb = r.initNearbyLb(servers)
}

func (r *NearbyLoadBalancer) GetServers() []Server {
	r.smu.RLock()
	defer r.smu.RUnlock()
	return r.allServers
}

func (r *NearbyLoadBalancer) initNearbyLb(servers []Server) nearbyLb {
	ret := nearbyLb{
		rlb: make(map[string]regionLb, 8),
	}
	// 全局列表
	balancer := &VersionLoadBalancer{
		LbPolicy: r.LbPolicy,
	}
	balancer.SetServers(servers)
	ret.alb = balancer
	// region划分
	serverMap := make(map[string]map[string][]Server, 8)
	for _, server := range servers {
		region := server.Region
		if region == "" {
			region = "#"
		}
		zone := server.Zone
		if zone == "" {
			zone = "#"
		}
		regionVal, b := serverMap[region]
		if !b {
			regionVal = make(map[string][]Server)
			serverMap[region] = regionVal
		}
		zoneVal, b := regionVal[zone]
		if !b {
			zoneVal = make([]Server, 0)
		}
		zoneVal = append(zoneVal, server)
		regionVal[zone] = zoneVal
	}
	for region, zoneServers := range serverMap {
		lb := regionLb{
			zlb: make(map[string]LoadBalancer, 8),
		}
		rServers := make([]Server, 0)
		for zone, zServers := range zoneServers {
			// region全量
			rServers = append(rServers, zServers...)
			// zone 列表
			balancer = &VersionLoadBalancer{
				LbPolicy: r.LbPolicy,
			}
			balancer.SetServers(zServers)
			lb.zlb[zone] = balancer
		}
		// region全量列表
		balancer = &VersionLoadBalancer{
			LbPolicy: r.LbPolicy,
		}
		balancer.SetServers(rServers)
		lb.alb = balancer
		ret.rlb[region] = lb
	}
	return ret
}

func (r *NearbyLoadBalancer) ChooseServer(ctx context.Context) (Server, error) {
	r.smu.RLock()
	defer r.smu.RUnlock()
	if len(r.allServers) == 0 {
		return Server{}, ServerNotFound
	}
	region := common.GetRegion()
	rlb, b := r.lb.rlb[region]
	if !b {
		return r.lb.alb.ChooseServer(ctx)
	}
	zone := common.GetZone()
	lb, b := rlb.zlb[zone]
	if !b {
		return rlb.alb.ChooseServer(ctx)
	}
	return lb.ChooseServer(ctx)
}
