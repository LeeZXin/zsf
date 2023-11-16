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

func Run(options ...Option) {
	startOnce.Do(func() {
		o := new(option)
		for _, opt := range options {
			opt(o)
		}
		if o.Banner != "" {
			fmt.Println(o.Banner)
		} else {
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
		}
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
		})
		afterInitialize()
		quit.Wait()
	})
}

type option struct {
	Banner string
}

type Option func(*option)

func WithBanner(banner string) Option {
	return func(o *option) {
		o.Banner = banner
	}
}
