package quit

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// 监听程序kill事件, 并执行注销函数
// 用于关闭资源等， 如httpServer，数据库等

type ShutdownHook func()

var (
	hookList = make([]ShutdownHook, 0)
	mu       = sync.Mutex{}
)

func AddShutdownHook(hook ShutdownHook) {
	if hook != nil {
		mu.Lock()
		defer mu.Unlock()
		hookList = append(hookList, hook)
	}
}

// Wait 注册signal事件，并无限等待
func Wait() {
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	mu.Lock()
	defer mu.Unlock()
	for _, fn := range hookList {
		fn()
	}
}
