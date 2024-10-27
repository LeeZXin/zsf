package main

import (
	"fmt"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/http/httptask"
	"github.com/LeeZXin/zsf/property/dynamic"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
)

func main() {
	dynamic.InitDefault()
	zsf.Run(
		zsf.WithDiscovery(discovery.NewEtcdDiscovery()),
		zsf.WithLifeCycles(
			httpserver.NewDefaultServer(
				httpserver.AddRouters(
					func(e *gin.Engine) {
						e.GET("/helloWorld", func(c *gin.Context) {
							c.String(http.StatusOK, "hello world")
						})
					},
					httptask.WithHttpTask(func() (string, httptask.Task) {
						return "helloWorld", func(_ []byte, _ url.Values) {
							fmt.Println("hello world")
						}
					}),
				),
				httpserver.WithRegistry(
					registry.NewDefaultEtcdRegistry(),
				),
				httpserver.WithEnableActuator(true),
				httpserver.WithEnablePromApi(true),
				httpserver.WithEnablePprof(true),
			),
		),
	)
}
