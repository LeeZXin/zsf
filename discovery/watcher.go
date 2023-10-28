package discovery

import (
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/psub"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"sync"
	"time"
)

// 一个协程定时获取节点地址变更

type AddrUpdateCallback func([]ServiceAddr)

type AddrWatcher struct {
	serviceMap sync.Map
	listener   *psub.Channel[[]ServiceAddr]
	ptask      *taskutil.PeriodicalTask
}

// OnChange 注册节点变更回调
func (w *AddrWatcher) OnChange(serviceName string, callback AddrUpdateCallback) {
	_ = w.listener.Subscribe(serviceName, func(addrs []ServiceAddr) {
		if addrs != nil {
			callback(addrs)
		}
	})
	//首次加载需要先获取服务列表
	addrs, err := GetServiceInfo(serviceName)
	if err == nil {
		callback(addrs)
	} else {
		callback([]ServiceAddr{})
	}
	w.serviceMap.Store(serviceName, addrs)
}

// watch 开启定时获取
func (w *AddrWatcher) watch() {
	w.serviceMap.Range(func(key, value any) bool {
		name := key.(string)
		newAddrs, err := GetServiceInfo(name)
		if err != nil {
			return false
		}
		if !CompareServiceAddr(value.([]ServiceAddr), newAddrs) {
			w.serviceMap.Store(name, newAddrs)
			_ = w.listener.Publish(name, newAddrs)
		}
		return true
	})
}

func (w *AddrWatcher) Shutdown() {
	w.listener.Shutdown()
	w.ptask.Stop()
}

func (w *AddrWatcher) Start() {
	w.ptask.Start()
}

func NewAddrWatcher() *AddrWatcher {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, executor.CallerRunsStrategy)
	channel, _ := psub.NewChannel[[]ServiceAddr](channelExecutor)
	w := &AddrWatcher{
		serviceMap: sync.Map{},
		listener:   channel,
	}
	w.ptask, _ = taskutil.NewPeriodicalTask(30*time.Second, w.watch)
	return w
}
