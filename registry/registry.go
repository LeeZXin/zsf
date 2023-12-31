package registry

import (
	"github.com/LeeZXin/zsf-utils/collections/hashmap"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"sync"
	"sync/atomic"
)

//服务发现

var (
	registryMap = hashmap.NewConcurrentHashMap[string, IRegistry]()

	httpService = atomic.Value{}
	grpcService = atomic.Value{}

	httpActive = atomic.Bool{}
	grpcActive = atomic.Bool{}

	httpMu = sync.Mutex{}
	grpcMu = sync.Mutex{}
)

const (
	ConsulRegistryType = "consul"
	MemRegistryType    = "mem"
	StaticRegistryType = "static"
)

func init() {
	RegisterServiceRegistry(&consulRegistry{})
	RegisterServiceRegistry(&memRegistry{})
	RegisterServiceRegistry(&staticRegistry{})
}

// IRegistry 插件式实现服务注册
type IRegistry interface {
	GetRegistryType() string
	RegisterSelf(ServiceInfo) DeregisterAction
}

type DeregisterAction interface {
	DeregisterSelf()
}

type deregisterActionImpl struct {
}

func (*deregisterActionImpl) DeregisterSelf() {}

// ServiceInfo 注册所需的信息
type ServiceInfo struct {
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight int
}

func (s *ServiceInfo) GetRpcName() string {
	return common.GetApplicationName() + "-" + s.Scheme
}

func RegisterServiceRegistry(registry IRegistry) {
	if registry == nil {
		return
	}
	registryMap.Put(registry.GetRegistryType(), registry)
}

func getServiceRegistry() (IRegistry, bool) {
	registryType := static.GetString("registry.type")
	if registryType == "" {
		registryType = ConsulRegistryType
	}
	return registryMap.Get(registryType)
}

type serviceWrapper struct {
	info       ServiceInfo
	deregister DeregisterAction
}

// RegisterHttpServer 注册http服务
func RegisterHttpServer() {
	if !static.GetBool("http.registry.enabled") {
		return
	}
	registry, b := getServiceRegistry()
	if !b {
		logger.Logger.Panic("unknown registry type")
	}
	// 先注销 再注册
	DeregisterHttpServer()
	httpMu.Lock()
	defer httpMu.Unlock()
	if httpActive.CompareAndSwap(false, true) {
		weight := static.GetInt("http.weight")
		if weight == 0 {
			weight = 1
		}
		info := ServiceInfo{
			Port:   common.HttpServerPort(),
			Scheme: common.HttpProtocol,
			Weight: weight,
		}
		deregisterAction := registry.RegisterSelf(info)
		httpService.Store(serviceWrapper{
			info:       info,
			deregister: deregisterAction,
		})
	}
}

// DeregisterHttpServer 注销http服务
func DeregisterHttpServer() {
	if !static.GetBool("http.registry.enabled") {
		return
	}
	httpMu.Lock()
	defer httpMu.Unlock()
	if httpActive.CompareAndSwap(true, false) {
		val := httpService.Load()
		if val == nil {
			return
		}
		val.(serviceWrapper).deregister.DeregisterSelf()
	}
}

// RegisterGrpcServer 注册grpc服务
func RegisterGrpcServer() {
	if !static.GetBool("grpc.registry.enabled") {
		return
	}
	registry, b := getServiceRegistry()
	if !b {
		logger.Logger.Panic("unknown registry type")
	}
	// 先注销 再注册
	DeregisterGrpcServer()
	grpcMu.Lock()
	defer grpcMu.Unlock()
	if grpcActive.CompareAndSwap(false, true) {
		weight := static.GetInt("grpc.weight")
		if weight == 0 {
			weight = 1
		}
		info := ServiceInfo{
			Port:   common.GrpcServerPort(),
			Scheme: common.GrpcProtocol,
			Weight: weight,
		}
		deregisterAction := registry.RegisterSelf(info)
		grpcService.Store(serviceWrapper{
			info:       info,
			deregister: deregisterAction,
		})
	}
}

// DeregisterGrpcServer 注销grpc服务
func DeregisterGrpcServer() {
	if !static.GetBool("grpc.registry.enabled") {
		return
	}
	grpcMu.Lock()
	defer grpcMu.Unlock()
	if grpcActive.CompareAndSwap(true, false) {
		val := grpcService.Load()
		if val == nil {
			return
		}
		val.(serviceWrapper).deregister.DeregisterSelf()
	}
}
