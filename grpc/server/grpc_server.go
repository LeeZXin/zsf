package grpcserver

import (
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/grpc/debug"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/registry"
	"github.com/LeeZXin/zsf/util/hashset"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"strings"
	"time"
)

// grpc server封装

const (
	DefaultServerPort = 15004
)

var (
	acceptedHeaders = make(hashset.HashSet[string])
)

func init() {
	h := property.GetString("grpc.server.acceptedHeaders")
	if h != "" {
		sp := strings.Split(h, ";")
		for _, s := range sp {
			acceptedHeaders.Add(s)
		}
	}
}

type Config struct {
	RegisterFunc             RegisterServiceFunc
	UnaryServerInterceptors  []grpc.UnaryServerInterceptor
	StreamServerInterceptors []grpc.StreamServerInterceptor
}

type RegisterServiceFunc func(server *grpc.Server)

// InitAndStartGrpcServer 开启grpc server
func InitAndStartGrpcServer(config Config) {
	// 端口信息
	port := property.GetInt("grpc.port")
	if port <= 0 {
		port = DefaultServerPort
	}
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Logger.Panic(err)
	}
	//开启grpc debug
	if property.GetBool("grpc.debug") {
		debug.StartGrpcDebug()
	}
	// unary拦截器
	uints := []grpc.UnaryServerInterceptor{
		headerUnaryInterceptor(),
		logErrorUnaryInterceptor(),
		prometheusUnaryInterceptor(),
		skywalkingUnaryInterceptor(),
	}
	if config.UnaryServerInterceptors != nil {
		uints = append(uints, config.UnaryServerInterceptors...)
	}
	// stream拦截器
	sints := []grpc.StreamServerInterceptor{
		headerStreamInterceptor(),
		logErrorStreamInterceptor(),
		prometheusStreamInterceptor(),
		skywalkingStreamInterceptor(),
	}
	if config.StreamServerInterceptors != nil {
		sints = append(sints, config.StreamServerInterceptors...)
	}
	// 连接空闲时间
	maxConnectionIdleSec := property.GetInt("grpc.maxConnectionIdleSec")
	if maxConnectionIdleSec == 0 {
		maxConnectionIdleSec = 600
	}
	// 请求超时时间 默认一分钟超时
	timeoutSec := property.GetInt("grpc.timeoutSec")
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
		grpc.ChainUnaryInterceptor(uints...),
		grpc.ChainStreamInterceptor(sints...),
	}
	if property.GetInt("grpc.maxRecvMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(property.GetInt("grpc.maxRecvMsgSize")))
	}
	if property.GetInt("grpc.maxSendMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(property.GetInt("grpc.maxSendMsgSize")))
	}
	// 是否进行服务注册
	if property.GetBool("grpc.registry.enabled") {
		weight := property.GetInt("grpc.weight")
		if weight == 0 {
			weight = 1
		}
		registry.RegisterSelf(
			registry.ServiceInfo{
				Port:   port,
				Scheme: common.GrpcProtocol,
				Weight: weight,
			},
		)
	}
	// 启动server
	server := grpc.NewServer(opts...)
	registerFunc := config.RegisterFunc
	if registerFunc != nil {
		registerFunc(server)
	}
	go func() {
		quit.AddShutdownHook(func() {
			logger.Logger.Info("grpc server shutdown")
			server.GracefulStop()
		})
		logger.Logger.Info("grpc server start:", port)
		err = server.Serve(listen)
		if err != nil {
			logger.Logger.Panic(err)
		}
	}()
}
