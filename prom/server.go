package prom

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

// 启动prometheus http服务，与正常httpServer隔离开

const (
	DefaultServerPort = 16054
)

type Server struct {
	httpServer *http.Server
}

func NewServer() *Server {
	return new(Server)
}

func (s *Server) Order() int {
	return 0
}

func (s *Server) OnApplicationStart() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.ContextWithFallback = true
	r.Any("/metrics", gin.WrapH(promhttp.Handler()))
	s.httpServer = &http.Server{
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
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalf("prometheus server starts failed: %v", err)
		}
	}()
}

func (s *Server) OnApplicationShutdown() {
	if s.httpServer != nil {
		logger.Logger.Info("prometheus server shutdown")
		_ = s.httpServer.Shutdown(context.Background())
	}
}

func (*Server) AfterInitialize() {}
