package httpserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/registry"
	"github.com/LeeZXin/zsf/zsf"
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

type server struct {
	enabled bool
	*http.Server
}

func (s *server) OnApplicationStart() {
	if !s.enabled {
		return
	}
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	e := gin.New()
	//静态资源文件路径
	e.Static("/static", filepath.Join(common.ResourcesDir, "static"))
	// 404
	noRoute := noRouteFunc.Load()
	if noRoute != nil {
		e.NoRoute(noRoute.(gin.HandlerFunc))
	}
	noMethod := noMethodFunc.Load()
	if noMethod != nil {
		e.NoMethod(noMethod.(gin.HandlerFunc))
	} else if noRoute != nil {
		e.NoMethod(noRoute.(gin.HandlerFunc))
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
	s.Server = &http.Server{
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
			logger.Logger.Panic("https.certFile config is empty")
		} else {
			certFilePath = filepath.Join(common.ResourcesDir, certFilePath)
		}
		if keyFilePath == "" {
			logger.Logger.Panic("https.keyFile config is empty")
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
			err = s.ListenAndServeTLS(certFilePath, keyFilePath)
		} else {
			logger.Logger.Info("http server start:", common.HttpServerPort())
			err = s.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Logger.Panic(err)
		}
	}()
}

func (s *server) AfterInitialize() {
	if !s.enabled {
		return
	}
	//是否开启http服务注册
	registry.RegisterHttpServer()
}

func (s *server) OnApplicationShutdown() {
	if !s.enabled {
		return
	}
	registry.DeregisterHttpServer()
	if s.Server != nil {
		logger.Logger.Info("http server shutdown")
		s.Shutdown(context.Background())
	}
}

func init() {
	zsf.RegisterApplicationLifeCycle(&server{
		enabled: !static.Exists("http.enabled") || static.GetBool("http.enabled"),
	})
}
