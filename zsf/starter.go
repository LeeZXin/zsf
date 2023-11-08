package zsf

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"sync"
)

var (
	startOnce sync.Once
)

func init() {
	_ = sentinel.InitDefault()
}

func Run() {
	startOnce.Do(func() {
		fmt.Print(`
 ████████  ████████ ████████
░░░░░░██  ██░░░░░░ ░██░░░░░ 
     ██  ░██       ░██      
    ██   ░█████████░███████ 
   ██    ░░░░░░░░██░██░░░░  
  ██            ░██░██      
 ████████ ████████ ░██      
░░░░░░░░ ░░░░░░░░  ░░   
:: zsf :: 
`)
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
		})
		afterInitialize()
		quit.Wait()
	})
}
