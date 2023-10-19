package discovery

import (
	"github.com/LeeZXin/zsf/common"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"strings"
	"sync"
)

var (
	discoveryMap = make(map[string]IDiscovery)
	discoveryMu  = sync.RWMutex{}
)

const (
	ConsulDiscoveryType = "consul"
	StaticDiscoveryType = "static"
	MemDiscoveryType    = "mem"
	EtcdV2DiscoveryType = "etcdV2"
)

func init() {
	RegisterServiceDiscovery(NewConsulDiscovery(nil))
	RegisterServiceDiscovery(&StaticDiscovery{})
	RegisterServiceDiscovery(&SaDiscovery{})
	endPoints := static.GetString("discovery.etcdV2.endPoints")
	username := static.GetString("discovery.etcdV2.username")
	password := static.GetString("discovery.etcdV2.password")
	if endPoints != "" {
		RegisterServiceDiscovery(NewEtcdV2Discovery(strings.Split(endPoints, ","), username, password))
	}
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
	ver := common.DefaultVersion
	for _, tag := range tags {
		if strings.HasPrefix(tag, common.VersionPrefix) {
			ver = strings.TrimPrefix(tag, common.VersionPrefix)
			break
		}
	}
	return ver
}

// GetServiceInfo 读取consul中的服务信息
func GetServiceInfo(name string) ([]ServiceAddr, error) {
	return GetServiceInfoByDiscoveryType(name, "")
}

// GetServiceInfoByDiscoveryType 读取consul中的服务信息
func GetServiceInfoByDiscoveryType(name, discoveryType string) ([]ServiceAddr, error) {
	if discoveryType == "" {
		discoveryType = static.GetString("discovery.type")
	}
	dis, ok := GetServiceDiscovery(discoveryType)
	if !ok {
		dis, _ = GetServiceDiscovery(StaticDiscoveryType)
	}
	return dis.GetServiceInfo(name)
}

func RegisterServiceDiscovery(discovery IDiscovery) {
	if discovery == nil {
		return
	}
	discoveryType := discovery.GetDiscoveryType()
	if discoveryType == "" {
		return
	}
	discoveryMu.Lock()
	defer discoveryMu.Unlock()
	discoveryMap[discoveryType] = discovery
}

func GetServiceDiscovery(discoveryType string) (IDiscovery, bool) {
	discoveryMu.RLock()
	defer discoveryMu.RUnlock()
	value, ok := discoveryMap[discoveryType]
	return value, ok
}
