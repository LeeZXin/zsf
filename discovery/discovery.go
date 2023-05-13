package discovery

import (
	"zsf/consul"
	_ "zsf/logger"
	"zsf/property"
)

// 服务发现

type ServiceAddress struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Version string `json:"version"`
}

type IDiscovery interface {
	GetServiceInfo(name string) ([]ServiceAddress, error)
}

func CheckServiceAddressesDiff(o, n []ServiceAddress) bool {
	if o == nil && n == nil {
		return true
	}
	if o == nil {
		return false
	} else if n == nil {
		return false
	}
	if len(o) != len(n) {
		return false
	}
	l := len(o)
	for i := 0; i < l; i++ {
		find := false
		for k := 0; k < l; k++ {
			if o[i].Address == n[k].Address && o[i].Port == n[k].Port &&
				o[i].Weight == n[k].Weight && o[i].Version == n[k].Version {
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

func init() {
	enabled := property.GetBool("discovery.enabled")
	if enabled {
		//加载服务发现
		impl = &consulDiscovery{
			client: consul.Client,
		}
	}
}
