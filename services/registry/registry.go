package registry

import (
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/lb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

// 服务注册
var (
	httpAction = newHttpAction()
	grpcAction = newGrpcAction()

	registryImpl Registry
	initOnce     = sync.Once{}
)

func getRegistry() Registry {
	initOnce.Do(func() {
		client, err := clientv3.New(clientv3.Config{
			Endpoints:        strings.Split(static.GetString("discovery.etcd.endpoints"), ";"),
			AutoSyncInterval: time.Minute,
			DialTimeout:      10 * time.Second,
			Username:         static.GetString("discovery.etcd.username"),
			Password:         static.GetString("discovery.etcd.password"),
			Logger:           zap.NewNop(),
		})
		if err != nil {
			logger.Logger.Fatalf("etcd client starts failed: %v", err)
		}
		quit.AddShutdownHook(func() {
			client.Close()
		})
		registryImpl = &etcdRegistry{
			client: client,
		}
	})
	return registryImpl
}

type RegisterAction struct {
	active     bool
	deregister DeregisterAction
	mu         sync.Mutex
	Enabled    bool
	Weight     int
	Port       int
	Scheme     string
}

func (r *RegisterAction) Register() {
	if !r.Enabled {
		return
	}
	r.Deregister()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		r.deregister = getRegistry().RegisterSelf(RegisterInfo{
			Port:   r.Port,
			Scheme: r.Scheme,
			Weight: r.Weight,
		})
		r.active = true
	}
}

func (r *RegisterAction) Deregister() {
	if !r.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		r.deregister()
		r.active = false
	}
}

func newHttpAction() *RegisterAction {
	weight := static.GetInt("http.weight")
	if weight == 0 {
		weight = 1
	}
	return &RegisterAction{
		Enabled: static.GetBool("http.registry.enabled"),
		Weight:  weight,
		Port:    common.HttpServerPort(),
		Scheme:  common.HttpProtocol,
	}
}

func newGrpcAction() *RegisterAction {
	weight := static.GetInt("grpc.weight")
	if weight == 0 {
		weight = 1
	}
	return &RegisterAction{
		Enabled: static.GetBool("grpc.registry.enabled"),
		Weight:  weight,
		Port:    common.GrpcServerPort(),
		Scheme:  common.GrpcProtocol,
	}
}

// Registry 插件式实现服务注册
type Registry interface {
	RegisterSelf(RegisterInfo) DeregisterAction
}

type DeregisterAction func()

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

func (s *RegisterInfo) GetServer() lb.Server {
	return lb.Server{
		Name:    s.GetRpcName(),
		Host:    common.GetLocalIP(),
		Port:    s.Port,
		Weight:  s.Weight,
		Version: env.GetVersion(),
		Region:  common.GetRegion(),
		Zone:    common.GetZone(),
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
	Port       int    `json:"Port"`
	Weight     int    `json:"Weight"`
	Version    string `json:"version"`
}
