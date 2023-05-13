package grpcclient

import (
	"context"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/executor"
	"github.com/LeeZXin/zsf/psub"
	"sync"
	"time"
)

// 一个协程定时获取grpc节点地址变更
// 而不是一个client一个协程去监听
type addressUpdateCallback func([]discovery.ServiceAddr)

type watcher struct {
	mu         sync.Mutex
	serviceMap map[string][]discovery.ServiceAddr
	listener   *psub.Channel
	cancelFunc context.CancelFunc
	ctx        context.Context
	ticker     time.Duration
}

func (w *watcher) Register(serviceName string, callback addressUpdateCallback) {
	_ = w.listener.Subscribe(serviceName, func(data any) {
		if data != nil {
			callback(data.([]discovery.ServiceAddr))
		}
	})
	//首次加载需要先获取服务列表
	addrs, err := discovery.GetServiceInfo(serviceName)
	if err == nil {
		callback(addrs)
	}
	w.mu.Lock()
	w.serviceMap[serviceName] = addrs
	w.mu.Unlock()
}

// Start 开启定时获取
// 锁过程拆成多段， 防止consul请求过长导致锁过长
func (w *watcher) Start() {
	go func() {
		for {
			ticker := time.NewTicker(w.ticker)
			select {
			case <-ticker.C:
				break
			case <-w.ctx.Done():
				ticker.Stop()
				return
			}
			w.mu.Lock()
			if len(w.serviceMap) == 0 {
				w.mu.Unlock()
				continue
			}
			services, addrs := w.copyService()
			w.mu.Unlock()
			//记录变更的服务
			changeNames := make([]string, 0)
			changeAddrs := make([][]discovery.ServiceAddr, 0)
			for i, service := range services {
				newAddrs, err := discovery.GetServiceInfo(service)
				if err != nil {
					break
				}
				oldArrs := addrs[i]
				if !discovery.DiffServiceAddresses(oldArrs, newAddrs) {
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

// 复制map数据 防止并发
func (w *watcher) copyService() ([]string, [][]discovery.ServiceAddr) {
	names := make([]string, 0, len(w.serviceMap))
	addrs := make([][]discovery.ServiceAddr, 0, len(w.serviceMap))
	for i, as := range w.serviceMap {
		names = append(names, i)
		addrs = append(addrs, as)
	}
	return names, addrs
}

func (w *watcher) Shutdown() {
	w.listener.Shutdown()
	w.cancelFunc()
}

func newWatcher(ticker time.Duration) *watcher {
	e, _ := executor.NewExecutor(2, 8, time.Minute, &executor.CallerRunsPolicy{})
	c, _ := psub.NewChannel(e)
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &watcher{
		mu:         sync.Mutex{},
		serviceMap: make(map[string][]discovery.ServiceAddr, 8),
		listener:   c,
		cancelFunc: cancelFunc,
		ctx:        ctx,
		ticker:     ticker,
	}
}
