package registry

import (
	"github.com/LeeZXin/zsf-utils/collections/hashmap"
	"github.com/LeeZXin/zsf/property/static"
)

//服务发现

var (
	registryMap = hashmap.NewConcurrentHashMap[string, IRegistry]()
)

const (
	ConsulRegistryType = "consul"
	MemRegistryType    = "mem"
)

func init() {
	RegisterServiceRegistry(&ConsulRegistry{})
	RegisterServiceRegistry(&MemRegistry{})
}

// IRegistry 插件式实现服务注册
type IRegistry interface {
	GetRegistryType() string
	StartRegisterSelf(ServiceInfo) IDeregister
}

type IDeregister interface {
	DeregisterSelf()
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

func RegisterServiceRegistry(registry IRegistry) {
	if registry == nil {
		return
	}
	registryMap.Put(registry.GetRegistryType(), registry)
}

func GetServiceRegistry() (IRegistry, bool) {
	registryType := static.GetString("registry.type")
	if registryType == "" {
		registryType = ConsulRegistryType
	}
	return registryMap.Get(registryType)
}
