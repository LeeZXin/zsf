package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/registry"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// gin服务封装
// 常见异常处理、header处理等
// 服务注册

type server struct {
	enabled bool
	*http.Server
}

func (s *server) OnApplicationStart() {
	if !s.enabled {
		return
	}
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	e := gin.New()
	//重写404请求
	e.NoRoute(http404)
	//filter
	e.Use(getFilters()...)
	fnList := getRegisterFuncList()
	for _, routerFunc := range fnList {
		routerFunc(e)
	}
	readTimeoutSec := static.GetInt("http.readTimeoutSec")
	if readTimeoutSec == 0 {
		readTimeoutSec = 20
	}
	writeTimeoutSec := static.GetInt("http.writeTimeoutSec")
	if writeTimeoutSec == 0 {
		writeTimeoutSec = 20
	}
	idleTimeoutSec := static.GetInt("http.idleTimeoutSec")
	if idleTimeoutSec == 0 {
		idleTimeoutSec = 60
	}
	s.Server = &http.Server{
		Addr:         fmt.Sprintf(":%d", common.DefaultHttpServerPort),
		ReadTimeout:  time.Duration(readTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(writeTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(idleTimeoutSec) * time.Second,
		Handler:      e,
	}
	logger.Logger.Info("http server start:", common.DefaultHttpServerPort)
	go func() {
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Panic(err)
		}
	}()
}

func (s *server) AfterInitialize() {
	if !s.enabled {
		return
	}
	//是否开启http服务注册
	registry.RegisterHttpServer()
}

func (s *server) OnApplicationShutdown() {
	if !s.enabled {
		return
	}
	registry.DeregisterHttpServer()
	if s.Server != nil {
		logger.Logger.Info("http server shutdown")
		s.Shutdown(context.Background())
	}
}

func http404(c *gin.Context) {
	c.JSON(http.StatusNotFound, "not found")
}

func init() {
	zsf.RegisterApplicationLifeCycle(&server{
		enabled: !static.Exists("http.enabled") || static.GetBool("http.enabled"),
	})
}
