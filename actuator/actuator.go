package actuator

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/ginutil"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
	"runtime"
	"time"
)

const (
	DefaultServerPort = 16055
)

type Server struct {
	httpServer *http.Server
}

func NewServer() *Server {
	return new(Server)
}

func (s *Server) OnApplicationStart() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.ContextWithFallback = true
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
	r.Any("/actuator/v1/markAsDownServer", func(c *gin.Context) {
		action := httpserver.GetRegistryAction()
		if action != nil {
			go action.MarkAsDown()
		}
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/markAsUpServer", func(c *gin.Context) {
		action := httpserver.GetRegistryAction()
		if action != nil {
			go action.MarkAsUp()
		}
		c.String(http.StatusOK, "ok")
	})
	s.httpServer = &http.Server{
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
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalf("actuator server starts failed: %v", err)
		}
	}()
}

func (s *Server) Order() int {
	return 0
}

func (s *Server) OnApplicationShutdown() {
	if s.httpServer != nil {
		logger.Logger.Info("actuator server shutdown")
		_ = s.httpServer.Shutdown(context.Background())
	}
}

func (*Server) AfterInitialize() {
}

type UpdateLogLevelReqVO struct {
	LogLevel string `json:"logLevel"`
}
