package zsf

import (
	"github.com/LeeZXin/zsf-utils/quit"
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
