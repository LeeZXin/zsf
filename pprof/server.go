package pprof

import (
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	enabled := static.GetBool("pprof.enabled")
	if enabled {
		port := static.GetInt("pprof.port")
		if port == 0 {
			logger.Logger.Panic("pprof port is empty")
		}
		//启动pprof server
		go func() {
			//只允许本地访问
			err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil)
			if err != nil && err != http.ErrServerClosed {
				logger.Logger.Panic(err)
			}
		}()
	}
}
