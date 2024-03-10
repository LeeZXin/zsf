package main

import (
	"github.com/LeeZXin/zsf/actuator"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	httpserver.AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.GET("/helloWorld", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
	})
	zsf.Run(
		zsf.WithDiscovery(discovery.NewStaticDiscovery()),
		zsf.WithLifeCycles(
			httpserver.NewServer(
				httpserver.WithRegistryAction(
					registry.NewDefaultHttpAction(
						registry.NewEtcdRegistry(),
					),
				),
			),
			actuator.NewServer(),
			prom.NewServer(),
			pprof.NewServer(),
		),
	)
}
