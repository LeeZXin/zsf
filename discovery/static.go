package discovery

import (
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"os"
)

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
	path := fmt.Sprintf("./resources/static-discovery-%s.json", cmd.GetEnv())
	content, err := os.ReadFile(path)
	if err != nil {
		path = "./resources/static-discovery.json"
		content, err = os.ReadFile(path)
		if err != nil {
			return
		}
	}
	logger.Logger.Info("load static discovery json path: ", path)
	var config staticConfig
	err = json.Unmarshal(content, &config)
	if err != nil {
		logger.Logger.Error("load static-discovery.json err:", err.Error())
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
				version := target.Version
				if version == "" {
					version = common.DefaultVersion
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
