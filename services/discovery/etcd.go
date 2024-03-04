package discovery

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/services/lb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

type etcdDiscovery struct {
	client clientv3.KV
	cmu    sync.RWMutex
	cache  map[string]lb.LoadBalancer
}

func (d *etcdDiscovery) Discover(name string) ([]lb.Server, error) {
	response, err := d.client.Get(context.Background(), common.ServicePrefix+name, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	servers := make([]lb.Server, 0, len(response.Kvs))
	for _, kv := range response.Kvs {
		var s lb.Server
		err = json.Unmarshal(kv.Value, &s)
		if err == nil {
			servers = append(servers, s)
		}
	}
	return servers, nil
}

func (d *etcdDiscovery) ChooseServer(ctx context.Context, name string) (lb.Server, error) {
	// 缓存地址信息
	d.cmu.RLock()
	loadBalancer, b := d.cache[name]
	d.cmu.RUnlock()
	if b {
		return loadBalancer.ChooseServer(ctx)
	}
	d.cmu.Lock()
	loadBalancer, err := d.getLoadBalancer(ctx, name)
	loadBalancer, b = d.cache[name]
	if b {
		d.cmu.Unlock()
		return loadBalancer.ChooseServer(ctx)
	}
	loadBalancer, err = d.getLoadBalancer(ctx, name)
	if err != nil {
		d.cmu.Unlock()
		return lb.Server{}, err
	}
	d.cache[name] = loadBalancer
	d.cmu.Unlock()
	return loadBalancer.ChooseServer(ctx)
}

func (d *etcdDiscovery) getLoadBalancer(ctx context.Context, name string) (lb.LoadBalancer, error) {
	servers, err := d.Discover(name)
	if err != nil {
		logger.Logger.WithContext(ctx).Error(err)
		return nil, err
	}
	balancer := &lb.NearbyLoadBalancer{
		LbPolicy: lb.Policy(lbPolicy),
	}
	balancer.SetServers(servers)
	return balancer, nil
}

func (d *etcdDiscovery) watch() {
	d.cmu.RLock()
	cpy := make(map[string][]lb.Server, len(d.cache))
	for name, loadBalancer := range d.cache {
		cpy[name] = loadBalancer.GetServers()
	}
	d.cmu.RUnlock()
	for name, servers := range cpy {
		newServers, err := d.Discover(name)
		if err != nil {
			logger.Logger.Error(err)
			return
		}
		if !compareServers(servers, newServers) {
			logger.Logger.Infof("server: %s has changed: %v", name, newServers)
			d.cache[name].SetServers(newServers)
		}
	}
}

func newEtcdDiscovery(endpoints, username, password string) *etcdDiscovery {
	d := new(etcdDiscovery)
	client, err := clientv3.New(clientv3.Config{
		Endpoints:        strings.Split(endpoints, ";"),
		AutoSyncInterval: time.Minute,
		DialTimeout:      10 * time.Second,
		Username:         username,
		Password:         password,
		Logger:           zap.NewNop(),
	})
	if err != nil {
		logger.Logger.Fatalf("etcd client starts failed: %v", err)
	}
	quit.AddShutdownHook(func() {
		client.Close()
	})
	d.client = clientv3.NewKV(client)
	d.cache = make(map[string]lb.LoadBalancer)
	watchTask, _ := taskutil.NewPeriodicalTask(10*time.Second, d.watch)
	watchTask.Start()
	quit.AddShutdownHook(watchTask.Stop)
	return d
}
