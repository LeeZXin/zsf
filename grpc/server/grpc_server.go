package grpcserver

import (
	"fmt"
	"github.com/LeeZXin/zsf/app"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/grpc/debug"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"
)

// grpc server封装

type Config struct {
	RegisterFunc             RegisterServiceFunc
	UnaryServerInterceptors  []grpc.UnaryServerInterceptor
	StreamServerInterceptors []grpc.StreamServerInterceptor
}

type RegisterServiceFunc func(server *grpc.Server)

// InitAndStartGrpcServer 开启grpc server
func InitAndStartGrpcServer(config Config) {
	port := property.GetInt("grpc.port")
	if port == 0 {
		logger.Logger.Panic("nil grpc port, fill it on application.yaml")
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Logger.Panic(err)
	}
	//开启grpc debug
	if property.GetBool("grpc.debug") {
		debug.StartGrpcDebug()
	}
	uints := []grpc.UnaryServerInterceptor{
		headerUnaryInterceptor(), logErrorUnaryInterceptor(),
		prometheusUnaryInterceptor(), skywalkingUnaryInterceptor(),
	}
	if config.UnaryServerInterceptors != nil {
		uints = append(uints, config.UnaryServerInterceptors...)
	}
	sints := []grpc.StreamServerInterceptor{
		headerStreamInterceptor(), logErrorStreamInterceptor(),
		prometheusStreamInterceptor(), skywalkingStreamInterceptor(),
	}
	if config.StreamServerInterceptors != nil {
		sints = append(sints, config.StreamServerInterceptors...)
	}
	maxConnectionIdleSec := property.GetInt("grpc.maxConnectionIdleSec")
	if maxConnectionIdleSec == 0 {
		maxConnectionIdleSec = 600
	}
	timeoutSec := property.GetInt("grpc.timeoutSec")
	if timeoutSec == 0 {
		timeoutSec = 600
	}
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: time.Duration(maxConnectionIdleSec) * time.Second,
			Timeout:           time.Duration(timeoutSec) * time.Second,
		}),
		grpc.ChainUnaryInterceptor(uints...),
		grpc.ChainStreamInterceptor(sints...),
	}
	if property.GetInt("grpc.maxRecvMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(property.GetInt("grpc.maxRecvMsgSize")))
	}
	if property.GetInt("grpc.maxSendMsgSize") > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(property.GetInt("grpc.maxSendMsgSize")))
	}
	if property.GetBool("grpc.registry.enabled") {
		weight := property.GetInt("grpc.weight")
		if weight == 0 {
			weight = 1
		}
		registry.RegisterSelf(registry.ServiceRegistryConfig{
			ApplicationName: app.ApplicationName,
			Ip:              common.LocalIP,
			Port:            port,
			Scheme:          common.GrpcScheme,
			Weight:          weight,
		})
	}
	server := grpc.NewServer(opts...)
	registerFunc := config.RegisterFunc
	if registerFunc != nil {
		registerFunc(server)
	}
	go func() {
		quit.RegisterQuitFunc(func() {
			logger.Logger.Info("grpc server shutdown")
			server.GracefulStop()
		})
		logger.Logger.Info("grpc server start:", port)
		err = server.Serve(lis)
		if err != nil {
			logger.Logger.Panic(err)
		}
	}()
}
