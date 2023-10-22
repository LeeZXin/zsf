package prom

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

// 启动prometheus http服务，与正常httpServer隔离开

func init() {
	enabled := static.GetBool("prometheus.enabled")
	if enabled {
		port := static.GetInt("prometheus.port")
		if port == 0 {
			logger.Logger.Panic("prometheus port is empty")
		}

		gin.SetMode(gin.ReleaseMode)
		r := gin.New()
		r.Any("/metrics", gin.WrapH(promhttp.Handler()))
		//启动promserver
		go func() {
			server := &http.Server{
				Addr:              fmt.Sprintf(":%d", port),
				ReadTimeout:       20 * time.Second,
				ReadHeaderTimeout: 20 * time.Second,
				WriteTimeout:      20 * time.Second,
				IdleTimeout:       time.Minute,
				Handler:           r,
			}
			logger.Logger.Info("prometheus server start: ", port)
			err := server.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				logger.Logger.Panic(err)
			}
			quit.AddShutdownHook(func() {
				_ = server.Shutdown(context.Background())
			})
		}()
	}
}
