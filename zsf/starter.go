package zsf

import (
	"github.com/LeeZXin/zsf/grpc/server"
	"github.com/LeeZXin/zsf/http/server"
	_ "github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/quit"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
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

func SetHttpRouter(f httpserver.RegisterRouterFunc) {
	httpRouter = f
}

func SetHttpFilters(filters []gin.HandlerFunc) {
	httpFilters = filters
}

func SetGrpcService(f grpcserver.RegisterServiceFunc) {
	grpcServices = f
}

func SetGrpcUnaryServerInterceptors(u []grpc.UnaryServerInterceptor) {
	grpcUnaryServerInterceptors = u
}

func SetGrpcStreamServerInterceptors(s []grpc.StreamServerInterceptor) {
	grpcStreamServerInterceptors = s
}

func Run() {
	startOnce.Do(func() {
		if static.GetBool("http.enabled") {
			httpserver.InitAndStartHttpServer(httpserver.Config{
				RegisterFunc: httpRouter,
				Filters:      httpFilters,
			})
		}
		if static.GetBool("grpc.enabled") {
			grpcserver.InitAndStartGrpcServer(grpcserver.Config{
				RegisterFunc:             grpcServices,
				UnaryServerInterceptors:  grpcUnaryServerInterceptors,
				StreamServerInterceptors: grpcStreamServerInterceptors,
			})
		}
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
		})
		quit.Wait()
	})
}
