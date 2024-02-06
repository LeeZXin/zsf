package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	"os"
	"path/filepath"
)

// 静态文件服务发现
type staticAddr struct {
	ServiceName string `json:"serviceName"`
	Targets     []struct {
		Addr    string `json:"addr"`
		Port    int    `json:"port"`
		Version string `json:"version"`
		Weight  int    `json:"weight"`
	} `json:"targets"`
}

type staticConfig struct {
	Static []staticAddr `json:"static,omitempty"`
}

type staticDiscovery struct {
	cache map[string][]ServiceAddr
	//多版本路由
	router map[string]map[string]selector.Selector[ServiceAddr]
}

func (s *staticDiscovery) Init() {
	path := fmt.Sprintf(filepath.Join(common.ResourcesDir, "static-discovery-%s.json"), env.GetEnv())
	content, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join(common.ResourcesDir, "static-discovery.json")
		content, err = os.ReadFile(path)
		if err != nil {
			return
		}
	}
	var config staticConfig
	err = json.Unmarshal(content, &config)
	if err != nil {
		return
	}
	s.cache = make(map[string][]ServiceAddr, 8)
	for _, service := range config.Static {
		if service.ServiceName == "" {
			continue
		}
		var addrs = make([]ServiceAddr, 0, len(service.Targets))
		for _, target := range service.Targets {
			if target.Addr == "" || target.Port == 0 {
				continue
			}
			if target.Version == "" {
				target.Version = common.DefaultVersion
			}
			addr := ServiceAddr{
				Addr:    target.Addr,
				Port:    target.Port,
				Version: target.Version,
				Weight:  target.Weight,
			}
			if addr.Weight <= 0 {
				addr.Weight = 1
			}
			addrs = append(addrs, addr)
		}
		s.cache[service.ServiceName] = addrs
	}
	s.router = make(map[string]map[string]selector.Selector[ServiceAddr], len(s.cache))
	for name, addrs := range s.cache {
		s.router[name] = convertToSelector(convertMultiVersionNodes(addrs), lbPolicy)
	}
}

func (*staticDiscovery) GetDiscoveryType() Type {
	return StaticDiscoveryType
}

func (s *staticDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	addrs, ok := s.cache[name]
	if ok {
		return addrs, nil
	}
	return []ServiceAddr{}, nil
}

func (s *staticDiscovery) PickOne(ctx context.Context, name string) (ServiceAddr, error) {
	multiVersionAddrs, b := s.router[name]
	if !b {
		return ServiceAddr{}, ServiceNotFound
	}
	targetSelector := findSelector(ctx, multiVersionAddrs)
	node, err := targetSelector.Select()
	if err == selector.EmptyNodesErr {
		return ServiceAddr{}, ServiceNotFound
	}
	return node.Data, nil
}

func (s *staticDiscovery) OnAddrChange(name string, changeFunc ServiceChangeFunc) {
	if changeFunc != nil {
		addrs, _ := s.GetServiceInfo(name)
		changeFunc(addrs)
	}
}
