package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
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
	action     registry.Action
	noRoute    gin.HandlerFunc
	noMethod   gin.HandlerFunc
	httpServer *http.Server
}

type opts struct {
	action   registry.Action
	noRoute  gin.HandlerFunc
	noMethod gin.HandlerFunc
}

type Opt func(*opts)

func WithRegistryAction(action registry.Action) Opt {
	return func(opts *opts) {
		opts.action = action
	}
}

func WithNoRoute(f gin.HandlerFunc) Opt {
	return func(opts *opts) {
		opts.noRoute = f
	}
}

func WithNoMethod(f gin.HandlerFunc) Opt {
	return func(opts *opts) {
		opts.noMethod = f
	}
}

func NewServer(os ...Opt) *Server {
	o := new(opts)
	for _, opt := range os {
		opt(o)
	}
	server = &Server{
		action:   o.action,
		noRoute:  o.noRoute,
		noMethod: o.noMethod,
	}
	return server
}

func (s *Server) GetRegistryAction() registry.Action {
	return s.action
}

func (s *Server) OnApplicationStart() {
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	e := gin.New()
	//静态资源文件路径
	e.Static("/static", filepath.Join(common.ResourcesDir, "static"))
	// 404
	if s.noRoute != nil {
		e.NoRoute(s.noRoute)
	}
	if s.noMethod != nil {
		e.NoMethod(s.noMethod)
	} else if s.noRoute != nil {
		e.NoMethod(s.noMethod)
	}
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
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", common.HttpServerPort()),
		ReadTimeout:  time.Duration(readTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(writeTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(idleTimeoutSec) * time.Second,
		Handler:      e,
		ErrorLog:     log.New(io.Discard, "", 0),
	}

	var (
		certFilePath string
		keyFilePath  string
	)
	httpsEnabled := static.GetBool("https.enabled")
	if httpsEnabled {
		certFilePath = static.GetString("https.certFile")
		keyFilePath = static.GetString("https.keyFile")
		if certFilePath == "" {
			logger.Logger.Fatal("https.certFile config is empty")
		} else {
			certFilePath = filepath.Join(common.ResourcesDir, certFilePath)
		}
		if keyFilePath == "" {
			logger.Logger.Fatal("https.keyFile config is empty")
		} else {
			keyFilePath = filepath.Join(common.ResourcesDir, keyFilePath)
		}
	}
	go func() {
		var err error
		if httpsEnabled {
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
	if s.action != nil {
		s.action.Register()
	}
}

func (s *Server) OnApplicationShutdown() {
	if s.action != nil {
		s.action.Deregister()
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
