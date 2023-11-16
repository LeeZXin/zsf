package actuator

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/ginutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/registry"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
	"runtime"
	"time"
)

func init() {
	zsf.RegisterApplicationLifeCycle(new(server))
}

const (
	DefaultServerPort = 16055
)

type server struct {
	*http.Server
}

func (s *server) OnApplicationStart() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 健康状态检查
	r.Any("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "")
	})
	// 触发gc
	r.Any("/actuator/v1/gc", func(c *gin.Context) {
		logger.Logger.WithContext(c.Request.Context()).Info("trigger gc")
		go runtime.GC()
		c.String(http.StatusOK, "")
	})
	// 更新日志level
	r.POST("/actuator/v1/updateLogLevel", func(c *gin.Context) {
		var reqVO UpdateLogLevelReqVO
		if ginutil.ShouldBind(&reqVO, c) {
			level := reqVO.LogLevel
			switch level {
			case "info":
				logger.Logger.SetLevel(logrus.InfoLevel)
				break
			case "debug":
				logger.Logger.SetLevel(logrus.DebugLevel)
				break
			case "warn":
				logger.Logger.SetLevel(logrus.WarnLevel)
				break
			case "error":
				logger.Logger.SetLevel(logrus.ErrorLevel)
				break
			case "trace":
				logger.Logger.SetLevel(logrus.TraceLevel)
				break
			default:
				break
			}
			c.String(http.StatusOK, "")
		}
	})
	r.Any("/actuator/v1/deregisterHttpServer", func(c *gin.Context) {
		go registry.DeregisterHttpServer()
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/registerHttpServer", func(c *gin.Context) {
		go registry.RegisterHttpServer()
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/deregisterGrpcServer", func(c *gin.Context) {
		go registry.DeregisterGrpcServer()
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/registerGrpcServer", func(c *gin.Context) {
		go registry.RegisterGrpcServer()
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/deregisterServer", func(c *gin.Context) {
		go registry.DeregisterGrpcServer()
		go registry.DeregisterHttpServer()
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/registerServer", func(c *gin.Context) {
		go registry.RegisterGrpcServer()
		go registry.RegisterHttpServer()
		c.String(http.StatusOK, "ok")
	})
	s.Server = &http.Server{
		Addr:              fmt.Sprintf(":%d", DefaultServerPort),
		ReadTimeout:       20 * time.Second,
		ReadHeaderTimeout: 20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       time.Minute,
		Handler:           r,
	}
	//启动server
	go func() {
		logger.Logger.Infof("actuator server start port: %d", DefaultServerPort)
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Panic(err)
		}
	}()
}

func (s *server) OnApplicationShutdown() {
	if s.Server != nil {
		logger.Logger.Info("actuator server shutdown")
		_ = s.Shutdown(context.Background())
	}
}

func (*server) AfterInitialize() {
}

type UpdateLogLevelReqVO struct {
	LogLevel string `json:"logLevel"`
}
