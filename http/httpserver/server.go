package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
)

// gin服务封装
// 常见异常处理、header处理等
// 服务注册

var (
	server *Server
)

type Server struct {
	opt         *option
	regiChanger atomic.Value
	httpServer  *http.Server
	up          atomic.Bool
}

type option struct {
	regi     registry.Registry
	noRoute  gin.HandlerFunc
	noMethod gin.HandlerFunc

	httpsEnabled bool
	certFilePath string
	keyFilePath  string

	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration

	disableUseH2C  bool
	enableActuator bool
	enablePromApi  bool
	enablePprof    bool
}

type Option func(*option)

func WithRegistry(regi registry.Registry) Option {
	return func(opt *option) {
		opt.regi = regi
	}
}

func WithEnableActuator(enableActuator bool) Option {
	return func(opt *option) {
		opt.enableActuator = enableActuator
	}
}

func WithEnablePromApi(enablePromApi bool) Option {
	return func(opt *option) {
		opt.enablePromApi = enablePromApi
	}
}

func WithEnablePprof(enablePprof bool) Option {
	return func(opt *option) {
		opt.enablePprof = enablePprof
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

func (s *Server) OnApplicationStart() {
	s.up.Store(true)
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
	// filter
	engine.Use(getFilters()...)
	// actuator
	if s.opt.enableActuator {
		logger.Logger.Info("http server enables actuator")
		s.enableActuator(engine)
	}
	// prom api
	if s.opt.enablePromApi {
		logger.Logger.Info("http server enables prometheus api")
		s.enablePromApi(engine)
	}
	// pprof
	if s.opt.enablePprof {
		logger.Logger.Info("http server enables pprof")
		s.enablePprof(engine)
	}
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
	var addr string
	host := static.GetString("http.host")
	if host != "" {
		addr = fmt.Sprintf("%s:%d", host, common.HttpServerPort())
	} else {
		addr = fmt.Sprintf(":%d", common.HttpServerPort())
	}
	s.httpServer = &http.Server{
		Addr:         addr,
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

func (s *Server) enableActuator(r *gin.Engine) {
	// 健康状态检查
	r.Any("/actuator/health", func(c *gin.Context) {
		c.String(http.StatusOK, "")
	})
	// 触发gc
	r.Any("/actuator/v1/gc", func(c *gin.Context) {
		logger.Logger.WithContext(c.Request.Context()).Info("trigger gc")
		go runtime.GC()
		c.String(http.StatusOK, "")
	})
	// 更新日志level
	r.PUT("/actuator/v1/updateLogLevel/:level", func(c *gin.Context) {
		switch c.Param("level") {
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
	})
	r.Any("/actuator/v1/markAsDownServer", func(c *gin.Context) {
		action := GetRegistryAction()
		if action != nil {
			go action.MarkAsDown()
		}
		c.String(http.StatusOK, "ok")
	})
	r.Any("/actuator/v1/markAsUpServer", func(c *gin.Context) {
		action := GetRegistryAction()
		if action != nil {
			go action.MarkAsUp()
		}
		c.String(http.StatusOK, "ok")
	})
}

func (s *Server) enablePromApi(r *gin.Engine) {
	r.Any("/metrics", gin.WrapH(promhttp.Handler()))
}

func (s *Server) enablePprof(r *gin.Engine) {
	r.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))
}

func (s *Server) AfterInitialize() {
	if s.opt.regi != nil {
		weight := static.GetInt("http.weight")
		if weight <= 0 {
			weight = 1
		}
		go func() {
			for s.up.Load() {
				isDown := false
				val := s.regiChanger.Load()
				if val != nil {
					isDown = val.(registry.StatusChanger).IsDown()
				}
				changer, err := s.opt.regi.Register(registry.ServerInfo{
					Port:     common.HttpServerPort(),
					Protocol: common.HttpProtocol,
					Weight:   weight,
				}, isDown)
				if err != nil {
					logger.Logger.Error(err)
				} else {
					s.regiChanger.Store(changer)
				}
				err = changer.KeepAlive()
				if err != nil && err != context.Canceled {
					logger.Logger.Error(err)
				}
				time.Sleep(5 * time.Second)
			}
		}()
	}
}

func (s *Server) OnApplicationShutdown() {
	statusChanger := s.regiChanger.Load()
	if statusChanger != nil {
		statusChanger.(registry.StatusChanger).Deregister()
	}
	if s.httpServer != nil {
		logger.Logger.Info("http server shutdown")
		ctx, fn := context.WithTimeout(context.Background(), 3*time.Second)
		defer fn()
		s.up.Store(false)
		s.httpServer.Shutdown(ctx)
	}
}

func GetRegistryAction() registry.StatusChanger {
	if server != nil {
		val := server.regiChanger.Load()
		if val != nil {
			return val.(registry.StatusChanger)
		}
	}
	return nil
}
