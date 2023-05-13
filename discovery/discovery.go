package discovery

import (
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/hashicorp/consul/api"
	"strings"
)

// ServiceAddr 服务信息
type ServiceAddr struct {
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Version string `json:"version"`
}

func DiffServiceAddr(oldAddr, newAddr []ServiceAddr) bool {
	if oldAddr == nil && newAddr == nil {
		return true
	}
	if oldAddr == nil {
		return false
	} else if newAddr == nil {
		return false
	}
	if len(oldAddr) != len(newAddr) {
		return false
	}
	addrLen := len(oldAddr)
	for i := 0; i < addrLen; i++ {
		find := false
		for k := 0; k < addrLen; k++ {
			if oldAddr[i].Addr == newAddr[k].Addr && oldAddr[i].Port == newAddr[k].Port &&
				oldAddr[i].Weight == newAddr[k].Weight && oldAddr[i].Version == newAddr[k].Version {
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

// findServiceTagVersion 读取tag中版本信息
func findServiceTagVersion(tags []string) string {
	ver := appinfo.DefaultVersion
	for _, tag := range tags {
		if strings.HasPrefix(tag, appinfo.VersionPrefix) {
			ver = strings.TrimPrefix(tag, appinfo.VersionPrefix)
			break
		}
	}
	return ver
}

// convert2ServiceAddr 转化为ServiceAddress
func convert2ServiceAddr(service *api.ServiceEntry) ServiceAddr {
	return ServiceAddr{
		Addr:    service.Service.Address,
		Port:    service.Service.Port,
		Weight:  service.Service.Weights.Passing,
		Version: findServiceTagVersion(service.Service.Tags),
	}
}

// GetServiceInfo 读取consul中的服务信息
func GetServiceInfo(name string) ([]ServiceAddr, error) {
	services, _, err := consul.GetConsulClient().Health().Service(name, "", true, nil)
	if err != nil {
		logger.Logger.Error(err)
		return nil, err
	}
	res := make([]ServiceAddr, 0, 8)
	for _, service := range services {
		address := convert2ServiceAddr(service)
		res = append(res, address)
	}
	return res, err
}
