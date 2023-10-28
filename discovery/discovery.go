package discovery

import (
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/common"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"strings"
	"sync"
)

var (
	discoveryMap = sync.Map{}
	watcher      = NewAddrWatcher()
	watcherOnce  = sync.Once{}
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
	RegisterServiceDiscovery(&MemDiscovery{})
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

func (s *ServiceAddr) IsSameAs(s2 *ServiceAddr) bool {
	if s2 == nil {
		return false
	}
	return s.Addr == s2.Addr && s.Port == s2.Port &&
		s.Weight == s2.Weight && s.Version == s2.Version
}

func CompareServiceAddr(oldAddr, newAddr []ServiceAddr) bool {
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
			if oldAddr[i].IsSameAs(&newAddr[k]) {
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
	discoveryMap.Store(discovery.GetDiscoveryType(), discovery)
}

func GetServiceDiscovery(discoveryType string) (IDiscovery, bool) {
	value, ok := discoveryMap.Load(discoveryType)
	if ok {
		return value.(IDiscovery), ok
	}
	return nil, ok
}

func OnAddrChange(serviceName string, callback AddrUpdateCallback) {
	watcherOnce.Do(func() {
		watcher.Start()
		quit.AddShutdownHook(func() {
			watcher.Shutdown()
		})
	})
	watcher.OnChange(serviceName, callback)
}
