package zsf

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"sync"
	"time"
)

var (
	startOnce sync.Once
	// 服务启动时间
	startTime = time.Now()
)

func init() {
	_ = sentinel.InitDefault()
}

func GetStartTime() time.Time {
	return startTime
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
