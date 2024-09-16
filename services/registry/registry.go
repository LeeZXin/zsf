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
	"time"
)

type StatusChanger interface {
	IsDown() bool
	MarkAsDown() error
	MarkAsUp() error
	Deregister() error
	KeepAlive() error
}

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

// Registry 插件式实现服务注册
type Registry interface {
	Register(ServerInfo, bool) (StatusChanger, error)
}

// ServerInfo 注册所需的信息
type ServerInfo struct {
	// Port 端口
	Port int
	// Protocol 服务协议
	Protocol string
	// Weight 权重
	Weight int

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
		s.rpcName = common.GetApplicationName() + "-" + s.Protocol
	}
	return s.rpcName
}

func (s *ServerInfo) GetServer(IsDown bool) lb.Server {
	return lb.Server{
		Protocol: s.Protocol,
		Name:     s.GetRpcName(),
		Host:     common.GetLocalIP(),
		Port:     s.Port,
		Weight:   s.Weight,
		Version:  env.GetVersion(),
		Region:   common.GetRegion(),
		Zone:     common.GetZone(),
		IsDown:   IsDown,
	}
}
