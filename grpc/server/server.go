// Package server 提供基于 gRPC 的 RPC 服务封装，负责创建和管理 gRPC 服务生命周期。
//
// 通过 NewDefaultServer + Option 构建服务实例，并作为 lifecycle.Object 接入框架
// 启动流程：
//   - OnApplicationStart：读取 grpc.port 配置、创建 grpc.Server（注册服务实现、
//     拦截器、健康检查、反射）并异步监听；端口非法、监听失败、注册器初始化
//     失败均直接 Fatal 退出进程
//   - AfterInitialize：服务已可访问后，向注册中心注册本实例（注册失败 Fatal）
//   - OnApplicationShutdown：注销注册中心并 GracefulStop；超时后 Stop 强制断开
//     （grpc.shutdown-timeout，未配置默认 30s）
//
// 职责边界：本包负责 gRPC 传输层与生命周期；拦截器（recover/header/prom/sentinel）
// 在同包 interceptor.go；业务服务实现由业务侧基于 .proto 生成代码提供，
// 通过 RegisterService 注册（参考 grpc/server/service 下的示例）。
package server

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/registry"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// 下载protoc https://github.com/protocolbuffers/protobuf/releases https://www.cnblogs.com/wylshkjj/p/16722735.html
// 安装protoc-gen-go-grpc go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0
// 安装protoc-gen-go go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
// protoc --go_out=. --go-grpc_out=. greeter.proto

// Option 服务配置选项函数类型，用于构建 Server。
type Option func(*options)

// Service 服务注册函数，入参为 *grpc.Server，业务在其中注册 .proto 生成的
// 服务实现（如 RegisterGreeterServiceServer），由 RegisterService 传入。
type Service func(*grpc.Server)

// WithGrpcPort 设置 gRPC 监听端口；未设置（<=0）时回退读取静态配置 grpc.port。
func WithGrpcPort(port int) Option {
	return func(opt *options) {
		opt.grpcPort = port
	}
}

// WithMaxRecvMsgSize 设置单条消息最大接收字节数；未设置时用 gRPC 默认值 4MB。
func WithMaxRecvMsgSize(size int) Option {
	return func(opt *options) {
		opt.maxRecvMsgSize = size
	}
}

// EnableHealthCheck 注册标准 gRPC 健康检查服务（grpc_health_v1），
// 供探针/注册中心健康探测使用；状态可通过 GetHealthServer 动态调整。
func EnableHealthCheck() Option {
	return func(opt *options) {
		opt.healthCheck = true
	}
}

// WithReflection 启用 gRPC 反射服务，方便 grpcurl 等工具在线调试。
func WithReflection() Option {
	return func(opt *options) {
		opt.reflection = true
	}
}

// WithNewRegistrarFunc 设置自定义服务注册中心工厂函数（协议固定为 grpc）。
// 默认不注册到注册中心；传入后启动流程自动创建注册器，并在
// AfterInitialize 注册、OnApplicationShutdown 注销（失败均 Fatal）。
func WithNewRegistrarFunc(f registry.NewRegistrarFunc) Option {
	return func(opt *options) {
		opt.newRegistrarFunc = f
	}
}

// WithName 设置服务名称，仅用于日志标识（如 "grpc server: xxx start"）。
func WithName(n string) Option {
	return func(opt *options) {
		opt.name = n
	}
}

// AddUnaryInterceptors 追加一元拦截器（NewDefaultServer 的默认拦截器链
// 在最外层，后追加的拦截器在其内层执行）。
func AddUnaryInterceptors(ints ...grpc.UnaryServerInterceptor) Option {
	return func(opt *options) {
		opt.unaryInts = append(opt.unaryInts, ints...)
	}
}

// AddStreamInterceptors 追加流式拦截器（顺序语义同 AddUnaryInterceptors）。
func AddStreamInterceptors(ints ...grpc.StreamServerInterceptor) Option {
	return func(opt *options) {
		opt.streamInts = append(opt.streamInts, ints...)
	}
}

// RegisterService 注册业务服务实现（每个 Service 在启动时对 grpc.Server
// 调用一次，具体注册方式见 Service 类型说明）。
func RegisterService(services ...Service) Option {
	return func(opt *options) {
		opt.services = services
	}
}

// Server gRPC 服务封装，负责创建和管理 grpc.Server 及生命周期。
// 生命周期方法（Order/OnApplicationStart/AfterInitialize/OnApplicationShutdown）
// 由 lifecycle 框架按启动顺序调用；OnApplicationStart 之后才可安全使用
// GetGrpcServer/GetHealthServer。
type Server struct {
	opts         *options
	grpcServer   *grpc.Server
	healthServer *health.Server
	registrar    registry.Registrar
}

type options struct {
	grpcPort int

	unaryInts  []grpc.UnaryServerInterceptor
	streamInts []grpc.StreamServerInterceptor

	maxRecvMsgSize int
	healthCheck    bool
	reflection     bool

	services []Service

	newRegistrarFunc registry.NewRegistrarFunc

	name string
}

// Order 返回服务启动顺序（优先级），0 表示默认优先级。
func (s *Server) Order() int {
	return 0
}

// OnApplicationStart 启动 gRPC 服务：端口未配置时读取静态配置 grpc.port；
// 创建 grpc.Server（注册服务实现、健康检查、反射，连接超时 30s）并异步
// Serve。端口非法、监听失败、注册器初始化失败均直接 Fatal 退出进程。
func (s *Server) OnApplicationStart() {
	grpcPort := s.opts.grpcPort
	if grpcPort <= 0 {
		grpcPort = static.GetInt("grpc.port")
	}
	if grpcPort <= 0 {
		logger.Logger.Fatal().Msgf("grpc server: %s port: %d is invalid", s.opts.name, grpcPort)
	}
	var addr string
	host := static.GetString("grpc.host")
	if host != "" {
		addr = fmt.Sprintf("%s:%d", host, grpcPort)
	} else {
		addr = fmt.Sprintf(":%d", grpcPort)
	}
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Logger.Fatal().Msgf("grpc server: %s failed to listen: %v", s.opts.name, err)
	}
	serverOptions := make([]grpc.ServerOption, 0)
	if s.opts.maxRecvMsgSize > 0 {
		serverOptions = append(serverOptions, grpc.MaxRecvMsgSize(s.opts.maxRecvMsgSize))
	}
	if len(s.opts.unaryInts) > 0 {
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(s.opts.unaryInts...))
	}
	if len(s.opts.streamInts) > 0 {
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(s.opts.streamInts...))
	}
	serverOptions = append(serverOptions, grpc.ConnectionTimeout(30*time.Second))
	s.grpcServer = grpc.NewServer(serverOptions...)
	for _, service := range s.opts.services {
		service(s.grpcServer)
	}
	if s.opts.healthCheck {
		s.healthServer = health.NewServer()
		healthpb.RegisterHealthServer(s.grpcServer, s.healthServer)
	}
	if s.opts.reflection {
		reflection.Register(s.grpcServer)
	}
	if s.opts.newRegistrarFunc != nil {
		s.registrar, err = s.opts.newRegistrarFunc(rpc.GrpcProtocol, grpcPort)
		if err != nil {
			logger.Logger.Fatal().Msgf("grpc server: %s failed to register registrar: %v", s.opts.name, err)
		}
	}
	go func() {
		logger.Logger.Info().Msgf("grpc server: %s start: %v", s.opts.name, addr)
		err := s.grpcServer.Serve(listen)
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			logger.Logger.Fatal().Msgf("grpc server: %s failed to serve: %v", s.opts.name, err)
		}
	}()
}

// AfterInitialize 应用初始化完成后向注册中心注册本服务实例
// （使服务可被发现）；注册失败直接 Fatal。
func (s *Server) AfterInitialize() {
	if s.registrar != nil {
		err := s.registrar.Register()
		if err != nil {
			logger.Logger.Fatal().Msgf("grpc server: %s failed to register registrar: %v", s.opts.name, err)
		}
	}
}

// OnApplicationShutdown 先注销注册中心，再 GracefulStop；
// 超时（grpc.shutdown-timeout，默认 30s）后 Stop 强制断开，避免拖到编排层 SIGKILL。
func (s *Server) OnApplicationShutdown() {
	if s.registrar != nil {
		s.registrar.Deregister()
	}
	if s.grpcServer != nil {
		logger.Logger.Info().Msgf("grpc server: %s shutdown", s.opts.name)
		d := quit.ShutdownTimeout(static.GetDuration("grpc.shutdown-timeout"))
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			logger.Logger.Warn().Msgf("grpc server: %s graceful stop timeout, force stop", s.opts.name)
			s.grpcServer.Stop()
			<-done
		}
	}
}

// GetGrpcServer 返回底层 grpc.Server 实例；OnApplicationStart 之前为 nil。
func (s *Server) GetGrpcServer() *grpc.Server {
	return s.grpcServer
}

// GetHealthServer 返回健康检查服务实例（需启用 EnableHealthCheck），
// 可通过 SetServingStatus 动态调整各服务/总体的健康状态。
func (s *Server) GetHealthServer() *health.Server {
	return s.healthServer
}

// NewDefaultServer 创建 Server，默认注册：
//   - 一元拦截器：recover（panic 兜底转 codes.Internal）、header（rpc 请求头
//     透传）、prom（耗时/频率埋点）
//   - 流拦截器：recover、header
//
// 可继续传入 Option 追加配置（如 RegisterService、WithGrpcPort）。
func NewDefaultServer(fns ...Option) *Server {
	defaultOpts := []Option{
		AddUnaryInterceptors(
			recoverUnaryInterceptor(),
			HeaderUnaryInterceptor(),
			promUnaryInterceptor(),
		),
		AddStreamInterceptors(
			recoverStreamInterceptor(),
			headerStreamInterceptor(),
		),
	}
	fns = append(defaultOpts, fns...)
	opts := &options{
		unaryInts:  make([]grpc.UnaryServerInterceptor, 0),
		streamInts: make([]grpc.StreamServerInterceptor, 0),
	}
	for _, fn := range fns {
		fn(opts)
	}
	return &Server{
		opts: opts,
	}
}
