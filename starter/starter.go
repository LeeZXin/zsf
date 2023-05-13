package starter

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"runtime"
	"sync"
	grpcclient "zsf/grpc/client"
	grpcserver "zsf/grpc/server"
	httpclient "zsf/http/client"
	httpserver "zsf/http/server"
	"zsf/logger"
	_ "zsf/pprof"
	"zsf/property"
	_ "zsf/property_loader"
	"zsf/quit"
)

var (
	httpRouter  httpserver.RegisterRouterFunc
	httpFilters []gin.HandlerFunc

	grpcServices                 grpcserver.RegisterServiceFunc
	grpcUnaryServerInterceptors  []grpc.UnaryServerInterceptor
	grpcStreamServerInterceptors []grpc.StreamServerInterceptor

	startOnce sync.Once
)

func RegisterHttpRouter(f httpserver.RegisterRouterFunc) {
	httpRouter = f
}

func SetHttpFilters(filters []gin.HandlerFunc) {
	httpFilters = filters
}

func RegisterGrpcService(f grpcserver.RegisterServiceFunc) {
	grpcServices = f
}

func SetGrpcUnaryServerInterceptors(u []grpc.UnaryServerInterceptor) {
	grpcUnaryServerInterceptors = u
}

func SetGrpcStreamServerInterceptors(s []grpc.StreamServerInterceptor) {
	grpcStreamServerInterceptors = s
}

func RegisterGrpcGlobalUnaryClientInterceptor(is ...grpc.UnaryClientInterceptor) {
	grpcclient.RegisterGlobalUnaryClientInterceptor(is...)
}

func RegisterHttpGlobalClientInterceptor(is ...httpclient.ClientInterceptor) {
	httpclient.RegisterGlobalClientInterceptor(is...)
}

func Run() {
	startOnce.Do(func() {
		logger.Logger.Info("runtime.GOMAXPROCS(0)=", runtime.GOMAXPROCS(0))
		if property.GetBool("http.enabled") {
			httpserver.InitAndStartHttpServer(httpserver.Config{
				RegisterRouterFunc: httpRouter,
				Filters:            httpFilters,
			})
		}
		if property.GetBool("grpc.enabled") {
			grpcserver.InitAndStartGrpcServer(grpcserver.Config{
				RegisterFunc:             grpcServices,
				UnaryServerInterceptors:  grpcUnaryServerInterceptors,
				StreamServerInterceptors: grpcStreamServerInterceptors,
			})
		}
		quit.Wait()
	})
}
