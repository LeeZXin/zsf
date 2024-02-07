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
	if static.GetBool("prometheus.enabled") {
		zsf.RegisterApplicationLifeCycle(new(server))
	}
}

const (
	DefaultServerPort = 16054
)

type server struct {
	*http.Server
}

func (s *server) OnApplicationStart() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Any("/metrics", gin.WrapH(promhttp.Handler()))
	s.Server = &http.Server{
		Addr:              fmt.Sprintf(":%d", DefaultServerPort),
		ReadTimeout:       20 * time.Second,
		ReadHeaderTimeout: 20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       time.Minute,
		Handler:           r,
	}
	//启动pprof server
	go func() {
		logger.Logger.Infof("prometheus server start port: %d", DefaultServerPort)
		err := s.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalf("prometheus server starts failed: %v", err)
		}
	}()
}

func (s *server) OnApplicationShutdown() {
	if s.Server != nil {
		logger.Logger.Info("prometheus server shutdown")
		_ = s.Shutdown(context.Background())
	}
}

func (*server) AfterInitialize() {
}
