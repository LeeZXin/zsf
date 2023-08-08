package zsf

import (
	"encoding/json"
	"github.com/LeeZXin/zsf/cmd"
	grpcclient "github.com/LeeZXin/zsf/grpc/client"
	grpcserver "github.com/LeeZXin/zsf/grpc/server"
	httpclient "github.com/LeeZXin/zsf/http/client"
	httpserver "github.com/LeeZXin/zsf/http/server"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/property"
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

func RegisterHttpGlobalClientInterceptor(is ...httpclient.Interceptor) {
	httpclient.RegisterInterceptor(is...)
}

func Run() {
	startOnce.Do(func() {
		loggerConfig := property.GetStringMap("logger")
		m, _ := json.Marshal(loggerConfig)
		loggerConfigJson := string(m)
		logger.Logger.Info("runtime.GOMAXPROCS(0)=", runtime.GOMAXPROCS(0))
		logger.Logger.Info("project version is ", cmd.GetVersion())
		logger.Logger.Info("project env is ", cmd.GetEnv())
		logger.Logger.Info("logger config:", loggerConfigJson)
		if property.GetBool("http.enabled") {
			httpserver.InitAndStartHttpServer(httpserver.Config{
				Register: httpRouter,
				Filters:  httpFilters,
			})
		}
		if property.GetBool("grpc.enabled") {
			grpcserver.InitAndStartGrpcServer(grpcserver.Config{
				RegisterFunc:             grpcServices,
				UnaryServerInterceptors:  grpcUnaryServerInterceptors,
				StreamServerInterceptors: grpcStreamServerInterceptors,
			})
		}
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
			logger.Logger.Println("Shutdown Server ...")
		})
		quit.Wait()
	})
}
