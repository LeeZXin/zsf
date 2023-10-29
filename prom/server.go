package prom

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

// 启动prometheus http服务，与正常httpServer隔离开

func init() {
	zsf.RegisterApplicationLifeCycle(&server{
		enabled: static.GetBool("prometheus.enabled"),
	})
}

const (
	DefaultPort = 16054
)

type server struct {
	enabled bool
	*http.Server
}

func (s *server) OnApplicationStart() {
	if !s.enabled {
		return
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Any("/metrics", gin.WrapH(promhttp.Handler()))
	port := static.GetInt("prometheus.port")
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
	//启动pprof server
	go func() {
		logger.Logger.Info("prometheus server start: ", port)
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Panic(err)
		}
	}()
}

func (s *server) OnApplicationShutdown() {
	if s.Server != nil {
		_ = s.Shutdown(context.Background())
	}
}

func (*server) AfterInitialize() {
}
