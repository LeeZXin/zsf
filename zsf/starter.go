package zsf

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"os"
	"path/filepath"
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

func init() {
	_ = sentinel.InitDefault()
}

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
		if o.Version != "" {
			version.Store(o.Version)
			logger.Logger.Info(fmt.Sprintf("start %s with version = %s ::", common.GetApplicationName(), o.Version))
		}
		if o.PidPath != "" {
			createPidFile(o.PidPath)
		}
		runMode.Store(o.RunMode)
		onApplicationStart()
		quit.AddShutdownHook(func() {
			onApplicationShutdown()
		})
		afterInitialize()
		quit.Wait()
	})
}

type option struct {
	Banner  string
	Version string
	PidPath string
	RunMode string
}

type Option func(*option)

func WithBanner(banner string) Option {
	return func(o *option) {
		o.Banner = banner
	}
}

func WithVersion(version string) Option {
	return func(o *option) {
		o.Version = version
	}
}

func WithPidFile(filePath string) Option {
	return func(o *option) {
		o.PidPath = filePath
	}
}

func WithRunMode(runMode string) Option {
	return func(o *option) {
		o.RunMode = runMode
	}
}

func createPidFile(filePath string) {
	currentPid := os.Getpid()
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		logger.Logger.Panic("create PID folder: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		logger.Logger.Panic("create PID file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(strconv.FormatInt(int64(currentPid), 10)); err != nil {
		logger.Logger.Panic("write PID information: %v", err)
	}
}
