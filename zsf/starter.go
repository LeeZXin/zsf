package zsf

import (
	"github.com/LeeZXin/zsf-utils/quit"
	_ "github.com/LeeZXin/zsf/pprof"
	"sync"
)

var (
	startOnce sync.Once
)

func Run() {
	startOnce.Do(func() {
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
		})
		afterInitialize()
		quit.Wait()
	})
}
