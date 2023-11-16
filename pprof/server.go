package pprof

import (
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/zsf"
	"net/http"
	_ "net/http/pprof"
)

const (
	DefaultServerPort = 16098
)

func init() {
	zsf.RegisterApplicationLifeCycle(new(server))
}

type server struct{}

func (*server) OnApplicationStart() {
	enabled := static.GetBool("pprof.enabled")
	if enabled {
		//启动pprof server
		go func() {
			//只允许本地访问
			err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", DefaultServerPort), nil)
			if err != nil && err != http.ErrServerClosed {
				logger.Logger.Panic(err)
			}
		}()
	}
}

func (*server) OnApplicationShutdown() {
}

func (*server) AfterInitialize() {
}
