package registry

import (
	"context"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/etcdclient"
	"github.com/LeeZXin/zsf/property/static"
	"sync"
)

// 服务注册
var (
	httpAction = newHttpAction()
	grpcAction = newGrpcAction()

	registryImpl Registry
)

func init() {
	if static.GetBool("http.registry.enabled") || static.GetBool("grpc.registry.enabled") {
		registryImpl = &etcdRegistry{
			client: etcdclient.GetClient(),
		}
	}
}

type registerAction struct {
	active     bool
	deregister DeregisterAction
	mu         sync.Mutex
	enabled    bool
	weight     int
	port       int
	scheme     string
}

func (r *registerAction) Register() {
	if !r.enabled {
		return
	}
	r.Deregister()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		r.deregister = registryImpl.RegisterSelf(RegisterInfo{
			Port:   r.port,
			Scheme: r.scheme,
			Weight: r.weight,
		})
		r.active = true
	}
}

func (r *registerAction) Deregister() {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		r.deregister()
		r.active = false
	}
}

func newHttpAction() *registerAction {
	weight := static.GetInt("http.weight")
	if weight == 0 {
		weight = 1
	}
	return &registerAction{
		enabled: static.GetBool("http.registry.enabled"),
		weight:  weight,
		port:    common.HttpServerPort(),
		scheme:  common.HttpProtocol,
	}
}

func newGrpcAction() *registerAction {
	weight := static.GetInt("grpc.weight")
	if weight == 0 {
		weight = 1
	}
	return &registerAction{
		enabled: static.GetBool("grpc.registry.enabled"),
		weight:  weight,
		port:    common.GrpcServerPort(),
		scheme:  common.GrpcProtocol,
	}
}

// Registry 插件式实现服务注册
type Registry interface {
	RegisterSelf(RegisterInfo) DeregisterAction
}

type DeregisterAction context.CancelFunc

// RegisterInfo 注册所需的信息
type RegisterInfo struct {
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight       int
	rpcName      string
	registerPath string
}

func (s *RegisterInfo) GetRegisterPath() string {
	if s.registerPath == "" {
		s.registerPath = common.ServicePrefix + s.GetRpcName() + "/" + common.GetInstanceId()
	}
	return s.registerPath
}

func (s *RegisterInfo) GetRpcName() string {
	if s.rpcName == "" {
		s.rpcName = common.GetApplicationName() + "-" + s.Scheme
	}
	return s.rpcName
}

func (s *RegisterInfo) GetServiceAddr() ServiceAddr {
	return ServiceAddr{
		InstanceId: common.GetInstanceId(),
		Name:       s.GetRpcName(),
		Addr:       common.GetLocalIP(),
		Port:       s.Port,
		Weight:     s.Weight,
		Version:    env.GetVersion(),
	}
}

// RegisterHttpServer 注册http服务
func RegisterHttpServer() {
	httpAction.Register()
}

// DeregisterHttpServer 注销http服务
func DeregisterHttpServer() {
	httpAction.Deregister()
}

// RegisterGrpcServer 注册grpc服务
func RegisterGrpcServer() {
	grpcAction.Register()
}

// DeregisterGrpcServer 注销grpc服务
func DeregisterGrpcServer() {
	grpcAction.Deregister()
}

// ServiceAddr 服务信息
type ServiceAddr struct {
	InstanceId string `json:"instanceId"`
	Name       string `json:"name"`
	Addr       string `json:"addr"`
	Port       int    `json:"port"`
	Weight     int    `json:"weight"`
	Version    string `json:"version"`
}
