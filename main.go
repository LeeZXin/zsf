package main

import (
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/starter"
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	httpserver.SetNoRouteFunc(func(c *gin.Context) {
		c.String(http.StatusNotFound, "fuc k")
	})
	httpserver.AppendRegisterRouterFunc(func(e *gin.Engine) {
		e.GET("/", func(c *gin.Context) {
			c.String(http.StatusOK, "hello world")
		})
	})
	starter.Run()
}
