package starter

import (
	grpcclient "github.com/LeeZXin/zsf/grpc/client"
	grpcserver "github.com/LeeZXin/zsf/grpc/server"
	httpclient "github.com/LeeZXin/zsf/http/client"
	httpserver "github.com/LeeZXin/zsf/http/server"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/property"
	_ "github.com/LeeZXin/zsf/property_loader"
	"github.com/LeeZXin/zsf/quit"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"runtime"
	"sync"
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
