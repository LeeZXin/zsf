package quit

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"zsf/logger"
	_ "zsf/logger"
)

// 监听程序kill事件, 并执行注销函数
// 用于关闭资源等， 如httpServer，数据库等

type QuitFunc func()

var (
	quitFuncList = make([]QuitFunc, 0)
	mutex        = sync.Mutex{}
)

func RegisterQuitFunc(quitFunc QuitFunc) {
	if quitFunc != nil {
		mutex.Lock()
		defer mutex.Unlock()
		quitFuncList = append(quitFuncList, quitFunc)
	}
}

// Wait 注册signal事件，并无限等待
func Wait() {
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	mutex.Lock()
	defer mutex.Unlock()
	for _, quitFunc := range quitFuncList {
		quitFunc()
	}
	logger.Logger.Println("Shutdown Server ...")
}
