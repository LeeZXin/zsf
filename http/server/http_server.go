package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/registry"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// gin服务封装
// 常见异常处理、header处理等
// 服务注册

type Config struct {
	Register RegisterRouterFunc
	Filters  []gin.HandlerFunc
}

type UpdateLogLevelRequest struct {
	LogLevel string `json:"logLevel,omitempty"`
}

type RegisterRouterFunc func(*gin.Engine)

func http404(c *gin.Context) {
	c.JSON(http.StatusNotFound, "pageNotFound")
}

// InitAndStartHttpServer 初始化http server
func InitAndStartHttpServer(config Config) {
	port := property.GetInt("http.port")
	if port == 0 {
		logger.Logger.Panic("nil http port, fill it on application.yaml first")
	}
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	r := gin.New()
	//重写404请求
	r.NoRoute(http404)
	//filter
	filters := []gin.HandlerFunc{
		recoverFilter(),
		headerFilter(),
		actuatorFilter(),
		promFilter(),
		skywalkingFilter(),
	}
	if config.Filters != nil {
		filters = append(filters, config.Filters...)
	}
	r.Use(filters...)
	if config.Register != nil {
		config.Register(r)
	}
	//是否开启http服务注册
	if property.GetBool("http.registry.enabled") {
		weight := property.GetInt("http.weight")
		if weight == 0 {
			weight = 1
		}
		//服务注册
		registry.RegisterSelf(registry.ServiceInfo{
			Port:   port,
			Scheme: common.HttpProtocol,
			Weight: weight,
		})
	}
	//启动httpserver
	go func() {
		readTimeoutSec := property.GetInt("http.readTimeoutSec")
		if readTimeoutSec == 0 {
			readTimeoutSec = 20
		}
		writeTimeoutSec := property.GetInt("http.writeTimeoutSec")
		if writeTimeoutSec == 0 {
			writeTimeoutSec = 20
		}
		idleTimeoutSec := property.GetInt("http.idleTimeoutSec")
		if idleTimeoutSec == 0 {
			idleTimeoutSec = 60
		}
		server := &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			ReadTimeout:  time.Duration(readTimeoutSec) * time.Second,
			WriteTimeout: time.Duration(writeTimeoutSec) * time.Second,
			IdleTimeout:  time.Duration(idleTimeoutSec) * time.Second,
			Handler:      r,
		}
		quit.AddShutdownHook(func() {
			logger.Logger.Info("http server shutdown")
			_ = server.Shutdown(context.Background())
		})
		logger.Logger.Info("http server start:", port)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Panic(err)
		}
	}()
}
