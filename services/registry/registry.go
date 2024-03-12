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

func NewDefaultEtcdRegistry() Registry {
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
	return &etcdRegistry{
		client: client,
	}
}

type Action interface {
	Register()
	Deregister()
}

type defaultRegisterAction struct {
	registry   Registry
	active     bool
	deregister DeregisterAction
	mu         sync.Mutex
	weight     int
	port       int
	scheme     string
}

func (r *defaultRegisterAction) Register() {
	r.Deregister()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		r.deregister = r.registry.RegisterSelf(ServerInfo{
			Port:   r.port,
			Scheme: r.scheme,
			Weight: r.weight,
		})
		r.active = true
	}
}

func (r *defaultRegisterAction) Deregister() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		r.deregister()
		r.active = false
	}
}

func NewDefaultHttpAction(registry Registry) Action {
	if registry == nil {
		logger.Logger.Fatalf("default http action gets nil registry")
	}
	weight := static.GetInt("http.weight")
	if weight <= 0 {
		weight = 1
	}
	return &defaultRegisterAction{
		registry: registry,
		weight:   weight,
		port:     common.HttpServerPort(),
		scheme:   common.HttpProtocol,
	}
}

// Registry 插件式实现服务注册
type Registry interface {
	RegisterSelf(ServerInfo) DeregisterAction
}

type DeregisterAction func()

// ServerInfo 注册所需的信息
type ServerInfo struct {
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight       int
	rpcName      string
	registerPath string
}

func (s *ServerInfo) GetRegisterPath() string {
	if s.registerPath == "" {
		s.registerPath = common.ServicePrefix + s.GetRpcName() + "/" + common.GetInstanceId()
	}
	return s.registerPath
}

func (s *ServerInfo) GetRpcName() string {
	if s.rpcName == "" {
		s.rpcName = common.GetApplicationName() + "-" + s.Scheme
	}
	return s.rpcName
}

func (s *ServerInfo) GetServer() lb.Server {
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

// ServiceAddr 服务信息
type ServiceAddr struct {
	InstanceId string `json:"instanceId"`
	Name       string `json:"name"`
	Addr       string `json:"addr"`
	Port       int    `json:"port"`
	Weight     int    `json:"weight"`
	Version    string `json:"version"`
}
