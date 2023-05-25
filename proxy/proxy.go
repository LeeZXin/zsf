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
)

var (
	TargetNotFoundErr = errors.New("target not found")
)

// RpcContext 反向代理上下文
type RpcContext struct {
	protocol string
	ctx      context.Context
	header   rpc.Header
	request  any
	picker   TargetServiceNamePicker
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

func (c *RpcContext) Picker() TargetServiceNamePicker {
	if c.picker == nil {
		return DefaultTargetServiceNamePicker
	}
	return c.picker
}

// newGrpcRpcContext 生成grpc的反向代理上下文
func newGrpcRpcContext(s grpc.ServerStream, picker TargetServiceNamePicker) *RpcContext {
	if picker == nil {
		picker = DefaultTargetServiceNamePicker
	}
	header := grpcserver.CopyIncomingContext(s.Context())
	ctx := rpc.SetHeaders(s.Context(), header)
	return &RpcContext{
		protocol: common.GrpcProtocol,
		ctx:      ctx,
		request:  s,
		header:   header,
		picker:   picker,
	}
}

// newHttpRpcContext 生成http的反向代理上下文
func newHttpRpcContext(c *gin.Context, picker TargetServiceNamePicker) *RpcContext {
	if picker == nil {
		picker = DefaultTargetServiceNamePicker
	}
	header := httpserver.CopyRequestHeader(c)
	ctx := rpc.SetHeaders(c.Request.Context(), header)
	return &RpcContext{
		protocol: common.HttpProtocol,
		ctx:      ctx,
		request:  c,
		header:   header,
		picker:   picker,
	}
}

// GrpcStreamProxy grpc代理
func GrpcStreamProxy(stream grpc.ServerStream, interceptors []Interceptor, picker TargetServiceNamePicker) error {
	rpcContext := newGrpcRpcContext(stream, picker)
	if interceptors == nil || len(interceptors) == 0 {
		return DoGrpcProxy(rpcContext)
	}
	wrapper := interceptorsWrapper{interceptorList: interceptors}
	return wrapper.intercept(rpcContext, DoGrpcProxy)
}

// HttpStreamProxy http代理
func HttpStreamProxy(stream *gin.Context, interceptors []Interceptor, picker TargetServiceNamePicker) error {
	rpcContext := newHttpRpcContext(stream, picker)
	if interceptors == nil || len(interceptors) == 0 {
		return DoHttpProxy(rpcContext)
	}
	wrapper := interceptorsWrapper{interceptorList: interceptors}
	return wrapper.intercept(rpcContext, DoHttpProxy)
}
