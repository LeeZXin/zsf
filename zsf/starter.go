package zsf

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	_ "github.com/LeeZXin/zsf-utils/sentinelutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/services/discovery"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	startOnce sync.Once
	// 服务启动时间
	startTime = time.Now()
	// 启动模式
	runMode = atomic.Value{}
	// 版本
	version = atomic.Value{}
)

func GetStartTime() time.Time {
	return startTime
}

func GetRunMode() string {
	val := runMode.Load()
	if val == nil {
		return ""
	}
	return val.(string)
}

func GetVersion() string {
	val := version.Load()
	if val == nil {
		return ""
	}
	return val.(string)
}

func Run(options ...Option) {
	startOnce.Do(func() {
		o := new(option)
		for _, opt := range options {
			opt(o)
		}
		if o.discovery != nil {
			discovery.SetDefaultDiscovery(o.discovery)
		}
		lifeCycles := o.LifeCycles
		if lifeCycles != nil {
			sort.SliceStable(lifeCycles, func(i, j int) bool {
				return lifeCycles[i].Order() < lifeCycles[j].Order()
			})
		}
		if o.Banner != "" {
			logger.Logger.Info(o.Banner)
		} else {
			logger.Logger.Info(`
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
		if o.Version != "" {
			version.Store(o.Version)
			logger.Logger.Info(fmt.Sprintf("start %s with version = %s ::", common.GetApplicationName(), o.Version))
		}
		if o.PidPath != "" {
			createPidFile(o.PidPath)
		}
		runMode.Store(o.RunMode)
		for _, l := range lifeCycles {
			l.OnApplicationStart()
		}
		quit.AddShutdownHook(func() {
			for _, l := range lifeCycles {
				l.OnApplicationShutdown()
			}
		}, true)
		for _, l := range lifeCycles {
			l.AfterInitialize()
		}
		quit.Wait()
	})
}

type option struct {
	Banner     string
	Version    string
	PidPath    string
	RunMode    string
	discovery  discovery.Discovery
	LifeCycles []LifeCycle
}

type Option func(*option)

func WithBanner(banner string) Option {
	return func(opt *option) {
		opt.Banner = banner
	}
}

func WithVersion(version string) Option {
	return func(opt *option) {
		opt.Version = version
	}
}

func WithPidFile(filePath string) Option {
	return func(opt *option) {
		opt.PidPath = filePath
	}
}

func WithRunMode(runMode string) Option {
	return func(opt *option) {
		opt.RunMode = runMode
	}
}

func WithDiscovery(d discovery.Discovery) Option {
	return func(opt *option) {
		opt.discovery = d
	}
}

func WithLifeCycles(lifeCycles ...LifeCycle) Option {
	return func(opt *option) {
		opt.LifeCycles = lifeCycles
	}
}

func createPidFile(filePath string) {
	currentPid := os.Getpid()
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		logger.Logger.Fatalf("create PID folder: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		logger.Logger.Fatalf("create PID file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(strconv.FormatInt(int64(currentPid), 10)); err != nil {
		logger.Logger.Fatalf("write PID information: %v", err)
	}
}
