package registry

import (
	"github.com/LeeZXin/zsf/property"
	"sync"
)

//服务发现
//目前只实现consul

var (
	registryMap = make(map[string]IRegistry)
	registryMu  = sync.RWMutex{}
)

const (
	ConsulRegistryType = "consul"
	SaRegistryType     = "sa"
)

func init() {
	NewServiceRegistry(&ConsulRegistry{})
	NewServiceRegistry(&SaRegistry{})
}

// IRegistry 插件式实现服务注册
type IRegistry interface {
	GetRegistryType() string
	StartRegisterSelf(ServiceInfo) error
}

// ServiceInfo 注册所需的信息
type ServiceInfo struct {
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight int
}

func RegisterSelf(info ServiceInfo) error {
	registryType := property.GetString("registry.type")
	if registryType == "" {
		registryType = ConsulRegistryType
	}
	r, ok := GetServiceRegistry(registryType)
	if ok {
		return r.StartRegisterSelf(info)
	}
	r, _ = GetServiceRegistry(ConsulRegistryType)
	return r.StartRegisterSelf(info)
}

func NewServiceRegistry(registry IRegistry) {
	if registry == nil {
		return
	}
	registryType := registry.GetRegistryType()
	if registryType == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registryMap[registryType] = registry
}

func GetServiceRegistry(registryType string) (IRegistry, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	value, ok := registryMap[registryType]
	return value, ok
}
