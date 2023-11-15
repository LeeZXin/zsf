package actuator

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/ginutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
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
	DefaultPort = 16055
)

type server struct {
	*http.Server
}

func (s *server) OnApplicationStart() {
	if !static.GetBool("actuator.enabled") {
		return
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// 健康状态检查
	r.Any("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "")
	})
	// 触发gc
	r.Any("/gc", func(c *gin.Context) {
		logger.Logger.WithContext(c.Request.Context()).Info("trigger gc")
		go runtime.GC()
		c.String(http.StatusOK, "")
	})
	// 更新日志level
	r.POST("/updateLogLevel", func(c *gin.Context) {
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
	port := static.GetInt("actuator.port")
	if port <= 0 {
		port = DefaultPort
	}
	s.Server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		ReadTimeout:       20 * time.Second,
		ReadHeaderTimeout: 20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       time.Minute,
		Handler:           r,
	}
	//启动server
	go func() {
		logger.Logger.Infof("actuator server start port: %d", port)
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
