package discovery

import (
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/appinfo"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/selector"
	"github.com/hashicorp/consul/api"
	"strconv"
	"strings"
	"sync"
)

var (
	discoveryMap = sync.Map{}
)

const (
	ConsulDiscoveryType = "consul"
)

func init() {
	NewServiceDiscovery(&ConsulDiscovery{})
}

type IDiscovery interface {
	GetDiscoveryType() string
	GetServiceInfo(name string) ([]ServiceAddr, error)
}

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
	discoveryType := property.GetString("discovery.type")
	if discoveryType == "" {
		discoveryType = ConsulDiscoveryType
	}
	dis, ok := GetServiceDiscovery(discoveryType)
	if ok {
		return dis.GetServiceInfo(name)
	}
	dis, _ = GetServiceDiscovery(ConsulDiscoveryType)
	return dis.GetServiceInfo(name)
}

func NewServiceDiscovery(discovery IDiscovery) {
	if discovery == nil {
		return
	}
	discoveryType := discovery.GetDiscoveryType()
	if discoveryType == "" {
		return
	}
	discoveryMap.Store(discoveryType, discovery)
}

func GetServiceDiscovery(discoveryType string) (IDiscovery, bool) {
	value, ok := discoveryMap.Load(discoveryType)
	if ok {
		return value.(IDiscovery), true
	}
	return nil, false
}

func ServiceMultiVersionNodes(serviceName string) (map[string][]selector.Node, error) {
	info, err := GetServiceInfo(serviceName)
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, errors.New("can not find ip address")
	}
	res := make(map[string][]selector.Node)
	//默认版本节点先初始化
	res[appinfo.DefaultVersion] = make([]selector.Node, 0)
	i := 0
	for _, item := range info {
		n := selector.Node{
			Id:     strconv.Itoa(i),
			Weight: item.Weight,
			Data:   fmt.Sprintf("%s:%d", item.Addr, item.Port),
		}
		version := appinfo.DefaultVersion
		if item.Version != "" {
			version = item.Version
		}
		ns, ok := res[version]
		if ok {
			res[version] = append(ns, n)
		} else {
			res[version] = append(make([]selector.Node, 0), n)
		}
		if version != appinfo.DefaultVersion {
			res[appinfo.DefaultVersion] = append(res[appinfo.DefaultVersion], n)
		}
		i += 1
	}
	return res, nil
}
