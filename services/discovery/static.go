package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/lb"
	"os"
	"path/filepath"
)

// 静态文件服务发现
type staticServer struct {
	Name    string `json:"name"`
	Targets []struct {
		Host    string `json:"host"`
		Port    int    `json:"port"`
		Version string `json:"version"`
		Weight  int    `json:"weight"`
		Region  string `json:"region"`
		Zone    string `json:"zone"`
	} `json:"targets"`
}

type staticConfig struct {
	Static []staticServer `json:"static,omitempty"`
}

type staticDiscovery struct {
	cache map[string][]lb.Server
	//多版本路由
	router map[string]lb.LoadBalancer
}

func NewStaticDiscovery() Discovery {
	ret := new(staticDiscovery)
	path := fmt.Sprintf(filepath.Join(common.ResourcesDir, "static-discovery-%s.json"), env.GetEnv())
	content, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join(common.ResourcesDir, "static-discovery.json")
		content, err = os.ReadFile(path)
		if err != nil {
			logger.Logger.Fatalf("can not find static-discovery.json: %v", err)
		} else {
			logger.Logger.Infof("read %s", path)
		}
	} else {
		logger.Logger.Infof("read %s", path)
	}
	var config staticConfig
	err = json.Unmarshal(content, &config)
	if err != nil {
		logger.Logger.Fatalf("can not read static-discovery.json: %v", err)
	}
	ret.cache = make(map[string][]lb.Server, 8)
	for _, staticServers := range config.Static {
		if staticServers.Name == "" {
			continue
		}
		servers := make([]lb.Server, 0, len(staticServers.Targets))
		for _, target := range staticServers.Targets {
			if target.Host == "" || target.Port == 0 {
				continue
			}
			if target.Version == "" {
				target.Version = common.DefaultVersion
			}
			server := lb.Server{
				Name:    staticServers.Name,
				Host:    target.Host,
				Port:    target.Port,
				Weight:  target.Weight,
				Version: target.Version,
				Region:  target.Region,
				Zone:    target.Zone,
			}
			if server.Weight <= 0 {
				server.Weight = 1
			}
			servers = append(servers, server)
		}
		ret.cache[staticServers.Name] = servers
	}
	lbPolicy := static.GetString("discovery.lbPolicy")
	ret.router = make(map[string]lb.LoadBalancer, len(ret.cache))
	for name, servers := range ret.cache {
		balancer := &lb.NearbyLoadBalancer{
			LbPolicy: lb.Policy(lbPolicy),
		}
		balancer.SetServers(servers)
		ret.router[name] = balancer
	}
	return ret
}

func (s *staticDiscovery) Discover(_ context.Context, name string) ([]lb.Server, error) {
	servers, ok := s.cache[name]
	if ok {
		return servers, nil
	}
	return nil, lb.ServerNotFound
}

func (s *staticDiscovery) DiscoverWithZone(context.Context, string, string) ([]lb.Server, error) {
	return nil, lb.ServerNotFound
}

func (s *staticDiscovery) ChooseServer(ctx context.Context, name string) (lb.Server, error) {
	balancer, b := s.router[name]
	if !b {
		return lb.Server{}, lb.ServerNotFound
	}
	return balancer.ChooseServer(ctx)
}

func (s *staticDiscovery) ChooseServerWithZone(context.Context, string, string) (lb.Server, error) {
	return lb.Server{}, lb.ServerNotFound
}
