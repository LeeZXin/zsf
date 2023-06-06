package grpcclient

import (
	"context"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/executor"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/psub"
	"sync"
	"time"
)

// 一个协程定时获取grpc节点地址变更
// 而不是一个client一个协程去监听

var (
	watchDuration int
)

func init() {
	// grpc服务发现间隔
	duration := property.GetInt("grpc.client.watchDuration")
	if duration <= 0 {
		duration = 30
	}
	watchDuration = duration
}

type addrUpdateCallback func([]discovery.ServiceAddr)

type serviceWatcher struct {
	mu         sync.RWMutex
	serviceMap map[string][]discovery.ServiceAddr
	listener   *psub.Channel
	cancelFunc context.CancelFunc
	ctx        context.Context
}

// OnChange 注册节点变更回调
func (w *serviceWatcher) OnChange(serviceName string, callback addrUpdateCallback) {
	_ = w.listener.Subscribe(serviceName, func(data any) {
		if data != nil {
			callback(data.([]discovery.ServiceAddr))
		}
	})
	//首次加载需要先获取服务列表
	addrs, err := discovery.GetServiceInfo(serviceName)
	if err == nil {
		callback(addrs)
	} else {
		callback(nil)
	}
	w.mu.Lock()
	w.serviceMap[serviceName] = addrs
	w.mu.Unlock()
}

// Start 开启定时获取
func (w *serviceWatcher) Start() {
	go func() {
		ticker := time.NewTicker(time.Duration(watchDuration) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				break
			case <-w.ctx.Done():
				return
			}
			w.mu.RLock()
			if len(w.serviceMap) == 0 {
				w.mu.RUnlock()
				continue
			}
			services, addrs := w.copyService()
			w.mu.RUnlock()

			//记录变更的服务
			changeNames := make([]string, 0)
			changeAddrs := make([][]discovery.ServiceAddr, 0)
			for i, service := range services {
				newAddrs, err := discovery.GetServiceInfo(service)
				if err != nil {
					break
				}
				oldArrs := addrs[i]
				if !discovery.DiffServiceAddr(oldArrs, newAddrs) {
					changeNames = append(changeNames, service)
					changeAddrs = append(changeAddrs, newAddrs)
				}
			}
			if len(changeNames) > 0 {
				for i, name := range changeNames {
					//通知节点变更
					_ = w.listener.Publish(name, changeAddrs[i])
				}
				w.mu.Lock()
				for i, name := range changeNames {
					//存储新节点
					w.serviceMap[name] = changeAddrs[i]
				}
				w.mu.Unlock()
			}
		}
	}()
}

// 复制map数据
func (w *serviceWatcher) copyService() ([]string, [][]discovery.ServiceAddr) {
	names := make([]string, 0, len(w.serviceMap))
	addrs := make([][]discovery.ServiceAddr, 0, len(w.serviceMap))
	for serviceName, serviceAddrs := range w.serviceMap {
		names = append(names, serviceName)
		addrs = append(addrs, serviceAddrs)
	}
	return names, addrs
}

func (w *serviceWatcher) Shutdown() {
	w.listener.Shutdown()
	w.cancelFunc()
}

func newWatcher() *serviceWatcher {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, &executor.CallerRunsPolicy{})
	channel, _ := psub.NewChannel(channelExecutor)
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &serviceWatcher{
		serviceMap: make(map[string][]discovery.ServiceAddr, 8),
		listener:   channel,
		cancelFunc: cancelFunc,
		ctx:        ctx,
	}
}
