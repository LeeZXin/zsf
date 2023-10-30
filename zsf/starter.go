package zsf

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	"sync"
)

var (
	startOnce sync.Once
)

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
