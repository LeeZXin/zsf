package grpcserver

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/collections/hashset"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/registry"
	"github.com/LeeZXin/zsf/zsf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"strings"
	"time"
)

// grpc server封装
var (
	acceptedHeaders = hashset.NewHashSet[string](nil)
)

func init() {
	h := static.GetString("grpc.server.acceptedHeaders")
	if h != "" {
		sp := strings.Split(h, ";")
		for _, s := range sp {
			acceptedHeaders.Add(s)
		}
	}
	zsf.RegisterApplicationLifeCycle(&server{
		enabled: static.GetBool("grpc.enabled"),
	})
}

type server struct {
	enabled bool
	*grpc.Server
}

func (s *server) OnApplicationStart() {
	if !s.enabled {
		return
	}
	// 连接空闲时间
	maxConnectionIdleSec := static.GetInt("grpc.maxConnectionIdleSec")
	if maxConnectionIdleSec == 0 {
		maxConnectionIdleSec = 600
	}
	// 请求超时时间 默认一分钟超时
	timeoutSec := static.GetInt("grpc.timeoutSec")
	if timeoutSec == 0 {
		timeoutSec = 60
	}
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(
			keepalive.ServerParameters{
				MaxConnectionIdle: time.Duration(maxConnectionIdleSec) * time.Second,
				Timeout:           time.Duration(timeoutSec) * time.Second,
			},
		),
		grpc.ChainUnaryInterceptor(getUnaryInterceptors()...),
		grpc.ChainStreamInterceptor(getStreamInterceptors()...),
	}
	if static.GetInt("grpc.maxRecvMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(static.GetInt("grpc.maxRecvMsgSize")))
	}
	if static.GetInt("grpc.maxSendMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(static.GetInt("grpc.maxSendMsgSize")))
	}
	// 启动server
	s.Server = grpc.NewServer(opts...)
	funcList := getRegisterFuncList()
	for _, fn := range funcList {
		fn(s.Server)
	}
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", common.GrpcServerPort))
	if err != nil {
		logger.Logger.Panic(err)
	}
	go func() {
		logger.Logger.Info("grpc server start:", common.GrpcServerPort)
		if err := s.Serve(listen); err != nil {
			logger.Logger.Panic(err)
		}
	}()
}

func (s *server) AfterInitialize() {
	if !s.enabled {
		return
	}
	// 是否进行服务注册
	registry.RegisterGrpcServer()
}

func (s *server) OnApplicationShutdown() {
	if !s.enabled {
		return
	}
	registry.DeregisterGrpcServer()
	if s.Server != nil {
		logger.Logger.Info("grpc server shutdown")
		s.GracefulStop()
	}
}
