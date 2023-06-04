package proxy

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/common"
	grpcserver "github.com/LeeZXin/zsf/grpc/server"
	httpserver "github.com/LeeZXin/zsf/http/server"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"sync"
)

var (
	TargetNotFoundErr      = errors.New("target not found")
	SourceNotFoundErr      = errors.New("source not found")
	ServiceCollisionErr    = errors.New("service collision")
	ModeNotFoundErr        = errors.New("proxy mode not found")
	NilRpcContextErr       = errors.New("nil rpc context")
	NilInvokerErr          = errors.New("nil invoker")
	NilGinCtxErr           = errors.New("nil gin.Context")
	NilGrpcServerStreamErr = errors.New("nil grpc.ServerStream")

	attachedService string
	attachedHost    string
	attachMu        = sync.RWMutex{}
)

const (
	// ModeSidecar sidecar模式
	ModeSidecar = "sideCar"
	// ModeProxy 代理模式
	ModeProxy = "proxy"
)

const (
	// AnyBoundTraffic 代理流量
	AnyBoundTraffic = iota
	// OutBoundTraffic 出口流量
	OutBoundTraffic
	// InBoundTraffic 入口流量
	InBoundTraffic
)

type Mode string

// RpcContext 反向代理上下文
type RpcContext struct {
	protocol      string
	ctx           context.Context
	header        rpc.Header
	request       any
	targetService string
	sourceService string
	trafficType   int
	attachedHost  string
}

func (c *RpcContext) Context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *RpcContext) SetContext(ctx context.Context) {
	if ctx != nil {
		c.ctx = ctx
	}
}

func (c *RpcContext) Request() any {
	return c.request
}

func (c *RpcContext) Header() rpc.Header {
	if c.header != nil {
		return c.header
	}
	return make(rpc.Header)
}

func (c *RpcContext) Protocol() string {
	return c.protocol
}

func (c *RpcContext) TargetService() string {
	return c.targetService
}

func (c *RpcContext) SourceService() string {
	return c.sourceService
}

func (c *RpcContext) TrafficType() int {
	return c.trafficType
}

// NewGrpcRpcContext 生成grpc的反向代理上下文
func NewGrpcRpcContext(s grpc.ServerStream) *RpcContext {
	header := grpcserver.CopyIncomingContext(s.Context())
	ctx := rpc.SetHeaders(s.Context(), header)
	return &RpcContext{
		protocol: common.GrpcProtocol,
		ctx:      ctx,
		request:  s,
		header:   header,
	}
}

// NewHttpRpcContext 生成http的反向代理上下文
func NewHttpRpcContext(c *gin.Context) *RpcContext {
	header := httpserver.CopyRequestHeader(c)
	ctx := rpc.SetHeaders(c.Request.Context(), header)
	return &RpcContext{
		protocol: common.HttpProtocol,
		ctx:      ctx,
		request:  c,
		header:   header,
	}
}

// RegisterAttachedService sidecar模式时注册的服务名称
func RegisterAttachedService(service, host string) {
	attachMu.Lock()
	defer attachMu.Unlock()
	attachedService = service
	attachedHost = host
}

// StreamProxy 执行代理
func StreamProxy(interceptors []Interceptor, proxyMode Mode, rpcContext *RpcContext, invoker Invoker) error {
	if rpcContext == nil {
		return NilRpcContextErr
	}
	if invoker == nil {
		return NilInvokerErr
	}
	targetServiceName, err := DefaultTargetServiceNamePicker(*rpcContext)
	if err != nil {
		return err
	}
	rpcContext.targetService = targetServiceName
	sourceServiceName, err := DefaultSourceServiceNamePicker(*rpcContext)
	if err != nil {
		return err
	}
	rpcContext.sourceService = sourceServiceName
	if sourceServiceName == targetServiceName {
		return ServiceCollisionErr
	}
	attachMu.RLock()
	switch proxyMode {
	case ModeSidecar:
		if sourceServiceName == attachedService {
			rpcContext.trafficType = OutBoundTraffic
		} else {
			rpcContext.trafficType = InBoundTraffic
			rpcContext.attachedHost = attachedHost
		}
		break
	case ModeProxy:
		rpcContext.trafficType = AnyBoundTraffic
		break
	default:
		return ModeNotFoundErr
	}
	attachMu.RUnlock()
	if interceptors == nil || len(interceptors) == 0 {
		return invoker(rpcContext)
	}
	wrapper := interceptorsWrapper{interceptorList: interceptors}
	return wrapper.intercept(rpcContext, invoker)
}

func HttpProxy(ginCtx *gin.Context, interceptors []Interceptor, proxyMode Mode) error {
	if ginCtx == nil {
		return NilGinCtxErr
	}
	rpcContext := NewHttpRpcContext(ginCtx)
	return StreamProxy(interceptors, proxyMode, rpcContext, DoHttpProxy)
}

func GrpcProxy(stream grpc.ServerStream, interceptors []Interceptor, proxyMode Mode) error {
	if stream == nil {
		return NilGrpcServerStreamErr
	}
	rpcContext := NewGrpcRpcContext(stream)
	return StreamProxy(interceptors, proxyMode, rpcContext, DoGrpcProxy)
}
