package discovery

import (
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"os"
	"path/filepath"
)

// 静态文件服务发现
var (
	cache = make(map[string][]ServiceAddr, 8)
)

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

func init() {
	path := fmt.Sprintf(filepath.Join(common.ResourcesDir, "static-discovery-%s.json"), cmd.GetEnv())
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
	static := config.Static
	if static != nil {
		for _, service := range static {
			serviceName := service.ServiceName
			targets := service.Targets
			if serviceName == "" {
				continue
			}
			var addrs = make([]ServiceAddr, 0, len(targets))
			for _, target := range targets {
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
				}
				if target.Weight > 0 {
					addr.Weight = target.Weight
				} else {
					addr.Weight = 1
				}
				addrs = append(addrs, addr)
			}
			cache[serviceName] = addrs
		}
	}
}

type StaticDiscovery struct{}

func (*StaticDiscovery) GetDiscoveryType() string {
	return StaticDiscoveryType
}

func (*StaticDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	addrs, ok := cache[name]
	if ok {
		return addrs, nil
	}
	return []ServiceAddr{}, nil
}
