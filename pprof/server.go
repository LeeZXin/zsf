package pprof

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"net/http"
	_ "net/http/pprof"
)

const (
	DefaultServerPort = 16098
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
	s.httpServer = &http.Server{
		Addr: fmt.Sprintf(":%d", DefaultServerPort),
	}
	//启动pprof server
	go func() {
		logger.Logger.Infof("pprof server start: %d", DefaultServerPort)
		//只允许本地访问
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalf("pprof server starts failed: %v", err)
		}
	}()
}

func (s *Server) OnApplicationShutdown() {
	if s.httpServer != nil {
		logger.Logger.Info("pprof server shutdown")
		s.httpServer.Shutdown(context.Background())
	}
}

func (*Server) AfterInitialize() {
}
