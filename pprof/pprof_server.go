package pprof

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"zsf/logger"
	"zsf/property"
)

func init() {
	enabled := property.GetBool("pprof.enabled")
	if enabled {
		port := property.GetInt("pprof.port")
		if port == 0 {
			logger.Logger.Panic("pprof port is empty")
		}
		//启动pprof server
		go func() {
			logger.Logger.Info("pprof server start: ", port)
			//只允许本地访问
			err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil)
			if err != nil {
				logger.Logger.Error(err)
			}
		}()
	}
}
