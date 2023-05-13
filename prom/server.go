package prom

import (
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

// 启动prometheus http服务，与正常httpServer隔离开

func init() {
	enabled := property.GetBool("prometheus.enabled")
	if enabled {
		port := property.GetInt("prometheus.port")
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
			if err != nil {
				logger.Logger.Error(err)
			}
		}()
	}
}
