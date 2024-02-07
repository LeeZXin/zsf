package discovery

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/collections/hashmap"
	"github.com/LeeZXin/zsf-utils/localcache"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/etcdclient"
	"github.com/LeeZXin/zsf/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"sync"
	"time"
)

type etcdDiscovery struct {
	initOnce   sync.Once
	watchOnce  sync.Once
	funcCache  hashmap.Map[string, ServiceChangeFunc]
	addrsCache hashmap.Map[string, []ServiceAddr]
	client     clientv3.KV
	router     localcache.ExpireCache[map[string]selector.Selector[ServiceAddr]]
}

func (*etcdDiscovery) GetDiscoveryType() string {
	return EtcdDiscoveryType
}

func (d *etcdDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	response, err := d.client.Get(context.Background(), common.ServicePrefix+name, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	ret := make([]ServiceAddr, 0, len(response.Kvs))
	for _, kv := range response.Kvs {
		var addr ServiceAddr
		err := json.Unmarshal(kv.Value, &addr)
		if err == nil {
			ret = append(ret)
		}
	}
	return ret, nil
}

func (d *etcdDiscovery) PickOne(ctx context.Context, name string) (ServiceAddr, error) {
	targetSelector, err := d.router.LoadData(ctx, name)
	if err != nil {
		return ServiceAddr{}, err
	}
	ret, err := findSelector(ctx, targetSelector).Select()
	if err == selector.EmptyNodesErr {
		return ServiceAddr{}, ServiceNotFound
	}
	return ret.Data, nil
}

func (d *etcdDiscovery) OnAddrChange(name string, changeFunc ServiceChangeFunc) {
	if changeFunc == nil {
		return
	}
	// 首次获取
	addrs, err := d.GetServiceInfo(name)
	if err != nil {
		logger.Logger.Error(err)
		return
	}
	// 执行回调
	changeFunc(addrs)
	// 缓存
	d.addrsCache.Put(name, addrs)
	d.funcCache.Put(name, changeFunc)
	d.watchOnce.Do(func() {
		// 每十秒获取判断是否不同 不使用watcher
		task, _ := taskutil.NewPeriodicalTask(10*time.Second, func() {
			cpy := d.addrsCache.ToMap()
			for srv, oldAddrs := range cpy {
				newAddrs, err := d.GetServiceInfo(srv)
				if err != nil {
					logger.Logger.Error(err)
					return
				}
				if !compareAddrs(oldAddrs, newAddrs) {
					d.addrsCache.Put(srv, newAddrs)
					fn, b := d.funcCache.Get(srv)
					if b {
						fn(newAddrs)
					}
				}
			}
		})
		task.Start()
		quit.AddShutdownHook(task.Stop, true)
	})
}

func (d *etcdDiscovery) Init() {
	d.funcCache = hashmap.NewConcurrentHashMap[string, ServiceChangeFunc]()
	d.addrsCache = hashmap.NewConcurrentHashMap[string, []ServiceAddr]()
	d.client = etcdclient.NewKV()
	d.router, _ = localcache.NewLocalCache(func(ctx context.Context, key string) (map[string]selector.Selector[ServiceAddr], error) {
		addrs, err := d.GetServiceInfo(key)
		if err != nil {
			logger.Logger.WithContext(ctx).Error(err)
			return nil, err
		}
		return convertToSelector(convertMultiVersionNodes(addrs), lbPolicy), nil
	}, 10*time.Second)
}
