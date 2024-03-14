package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

// gin服务封装
// 常见异常处理、header处理等
// 服务注册

var (
	server *Server
)

type Server struct {
	opt        *option
	httpServer *http.Server
}

type option struct {
	action   registry.Action
	noRoute  gin.HandlerFunc
	noMethod gin.HandlerFunc

	httpsEnabled bool
	certFilePath string
	keyFilePath  string

	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration

	disableUseH2C bool
}

type Option func(*option)

func WithRegistryAction(action registry.Action) Option {
	return func(opt *option) {
		opt.action = action
	}
}

func WithNoRoute(f gin.HandlerFunc) Option {
	return func(opt *option) {
		opt.noRoute = f
	}
}

func WithNoMethod(f gin.HandlerFunc) Option {
	return func(opt *option) {
		opt.noMethod = f
	}
}

func WithReadTimeout(t time.Duration) Option {
	return func(opt *option) {
		opt.readTimeout = t
	}
}

func WithWriteTimeout(t time.Duration) Option {
	return func(opt *option) {
		opt.writeTimeout = t
	}
}

func WithIdleTimeout(t time.Duration) Option {
	return func(opt *option) {
		opt.idleTimeout = t
	}
}

func WithDisableUseH2C() Option {
	return func(opt *option) {
		opt.disableUseH2C = true
	}
}

func WithHttpsEnabled(certFilePath, keyFilePath string) Option {
	return func(opt *option) {
		opt.httpsEnabled = true
		opt.certFilePath = certFilePath
		opt.keyFilePath = keyFilePath
	}
}

func NewServer(opts ...Option) *Server {
	opt := new(option)
	for _, apply := range opts {
		apply(opt)
	}
	server = &Server{
		opt: opt,
	}
	return server
}

func (s *Server) Order() int {
	return 0
}

func (s *Server) GetRegistryAction() registry.Action {
	return s.opt.action
}

func (s *Server) OnApplicationStart() {
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	engine := gin.New()
	if !s.opt.disableUseH2C {
		engine.UseH2C = true
	}
	engine.ContextWithFallback = true
	//静态资源文件路径
	engine.Static("/static", filepath.Join(common.ResourcesDir, "static"))
	// 404
	if s.opt.noRoute != nil {
		engine.NoRoute(s.opt.noRoute)
	}
	if s.opt.noMethod != nil {
		engine.NoMethod(s.opt.noMethod)
	} else if s.opt.noRoute != nil {
		engine.NoMethod(s.opt.noRoute)
	}
	engine.ContextWithFallback = true
	//filter
	engine.Use(getFilters()...)
	fnList := getRegisterFuncList()
	for _, routerFunc := range fnList {
		routerFunc(engine)
	}
	readTimeout := s.opt.readTimeout
	if readTimeout == 0 {
		readTimeout = 20 * time.Second
	}
	writeTimeout := s.opt.writeTimeout
	if writeTimeout == 0 {
		writeTimeout = 20 * time.Second
	}
	idleTimeout := s.opt.idleTimeout
	if idleTimeout == 0 {
		idleTimeout = 60 * time.Second
	}
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", common.HttpServerPort()),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
		Handler:      engine.Handler(),
		ErrorLog:     log.New(io.Discard, "", 0),
	}
	var (
		certFilePath string
		keyFilePath  string
	)
	if s.opt.httpsEnabled {
		certFilePath = s.opt.certFilePath
		keyFilePath = s.opt.keyFilePath
		if certFilePath == "" {
			logger.Logger.Fatal("https.certFile is empty")
		} else {
			certFilePath = filepath.Join(common.ResourcesDir, certFilePath)
		}
		if keyFilePath == "" {
			logger.Logger.Fatal("https.keyFile is empty")
		} else {
			keyFilePath = filepath.Join(common.ResourcesDir, keyFilePath)
		}
	}
	go func() {
		var err error
		if s.opt.httpsEnabled {
			logger.Logger.Info("https server start:", common.HttpServerPort())
			logger.Logger.Infof("https server certFile path: %s", certFilePath)
			logger.Logger.Infof("https server keyFile path: %s", keyFilePath)
			err = s.httpServer.ListenAndServeTLS(certFilePath, keyFilePath)
		} else {
			logger.Logger.Info("http server start:", common.HttpServerPort())
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalf("http server starts failed: %v", err)
		}
	}()
}

func (s *Server) AfterInitialize() {
	if s.opt.action != nil {
		s.opt.action.Register()
	}
}

func (s *Server) OnApplicationShutdown() {
	if s.opt.action != nil {
		s.opt.action.Deregister()
	}
	if s.httpServer != nil {
		logger.Logger.Info("http server shutdown")
		s.httpServer.Shutdown(context.Background())
	}
}

func GetRegistryAction() registry.Action {
	if server != nil {
		return server.GetRegistryAction()
	}
	return nil
}
