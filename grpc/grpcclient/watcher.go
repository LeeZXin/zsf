package grpcclient

import (
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/psub"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/discovery"
	"sync"
	"time"
)

// 一个协程定时获取grpc节点地址变更
// 而不是一个client一个协程去监听

type addrUpdateCallback func([]discovery.ServiceAddr)

type serviceWatcher struct {
	mu         sync.Mutex
	serviceMap map[string][]discovery.ServiceAddr
	listener   *psub.Channel[[]discovery.ServiceAddr]

	ptask *taskutil.PeriodicalTask
}

// OnChange 注册节点变更回调
func (w *serviceWatcher) OnChange(serviceName string, callback addrUpdateCallback) {
	_ = w.listener.Subscribe(serviceName, func(addrs []discovery.ServiceAddr) {
		if addrs != nil {
			callback(addrs)
		}
	})
	//首次加载需要先获取服务列表
	addrs, err := discovery.GetServiceInfo(serviceName)
	if err == nil {
		callback(addrs)
	} else {
		callback([]discovery.ServiceAddr{})
	}
	w.mu.Lock()
	w.serviceMap[serviceName] = addrs
	w.mu.Unlock()
}

// Start 开启定时获取
func (w *serviceWatcher) Start() {
	w.ptask.Start()
}

// watch 开启定时获取
func (w *serviceWatcher) watch() {
	serviceMap := w.copyServiceMap()
	if len(serviceMap) == 0 {
		return
	}
	//记录变更的服务
	changeNames := make([]string, 0)
	changeAddrs := make([][]discovery.ServiceAddr, 0)
	for name, oldArrs := range serviceMap {
		newAddrs, err := discovery.GetServiceInfo(name)
		if err != nil {
			break
		}
		if !discovery.DiffServiceAddr(oldArrs, newAddrs) {
			changeNames = append(changeNames, name)
			changeAddrs = append(changeAddrs, newAddrs)
		}
	}
	if len(changeNames) > 0 {
		for i, name := range changeNames {
			//通知节点变更
			_ = w.listener.Publish(name, changeAddrs[i])
			serviceMap[name] = changeAddrs[i]
		}
		w.setServiceMap(serviceMap)
	}
}

// 复制map数据
func (w *serviceWatcher) copyServiceMap() map[string][]discovery.ServiceAddr {
	w.mu.Lock()
	defer w.mu.Unlock()
	ret := make(map[string][]discovery.ServiceAddr, len(w.serviceMap))
	for k, v := range w.serviceMap {
		ret[k] = v
	}
	return ret
}

func (w *serviceWatcher) setServiceMap(data map[string][]discovery.ServiceAddr) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.serviceMap = data
}

func (w *serviceWatcher) Shutdown() {
	w.listener.Shutdown()
	w.ptask.Stop()
}

func newWatcher() *serviceWatcher {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, executor.CallerRunsStrategy)
	channel, _ := psub.NewChannel[[]discovery.ServiceAddr](channelExecutor)
	w := &serviceWatcher{
		serviceMap: make(map[string][]discovery.ServiceAddr, 8),
		listener:   channel,
	}
	w.ptask, _ = taskutil.NewPeriodicalTask(10*time.Second, w.watch)
	return w
}
