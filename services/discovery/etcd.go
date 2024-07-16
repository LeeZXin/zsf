package discovery

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/lb"
	"github.com/spf13/cast"
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

func (d *etcdDiscovery) Discover(ctx context.Context, name string) ([]lb.Server, error) {
	response, err := d.client.Get(ctx, common.ServicePrefix+name+"/", clientv3.WithPrefix())
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

func (d *etcdDiscovery) DiscoverWithZone(context.Context, string, string) ([]lb.Server, error) {
	return nil, lb.ServerNotFound
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
	loadBalancer, b = d.cache[name]
	if !b {
		loadBalancer, err := d.newLoadBalancer(ctx, name)
		if err != nil {
			d.cmu.Unlock()
			return lb.Server{}, err
		}
		d.cache[name] = loadBalancer
	}
	d.cmu.Unlock()
	return loadBalancer.ChooseServer(ctx)
}

func (d *etcdDiscovery) ChooseServerWithZone(context.Context, string, string) (lb.Server, error) {
	return lb.Server{}, lb.ServerNotFound
}

func (d *etcdDiscovery) newLoadBalancer(ctx context.Context, name string) (lb.LoadBalancer, error) {
	servers, err := d.Discover(ctx, name)
	if err != nil {
		logger.Logger.WithContext(ctx).Error(err)
		return nil, err
	}
	lbPolicy := static.GetString("discovery.lbPolicy")
	balancer := &lb.NearbyLoadBalancer{
		LbPolicy: lb.Policy(lbPolicy),
	}
	balancer.SetServers(servers)
	return balancer, nil
}

func (d *etcdDiscovery) watch(ctx context.Context) {
	d.cmu.RLock()
	cpy := make(map[string][]lb.Server, len(d.cache))
	for name, loadBalancer := range d.cache {
		cpy[name] = loadBalancer.GetServers()
	}
	d.cmu.RUnlock()
	for name, servers := range cpy {
		newServers, err := d.Discover(ctx, name)
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

type EtcdConfig struct {
	Endpoints, Username, Password, Zone string
}

func NewEtcdDiscovery() Discovery {
	return newEtcdDiscovery(EtcdConfig{
		Endpoints: static.GetString("discovery.etcd.endpoints"),
		Username:  static.GetString("discovery.etcd.username"),
		Password:  static.GetString("discovery.etcd.password"),
	})
}

func newEtcdDiscovery(cfg EtcdConfig) Discovery {
	d := new(etcdDiscovery)
	client, err := clientv3.New(clientv3.Config{
		Endpoints:        strings.Split(cfg.Endpoints, ";"),
		AutoSyncInterval: time.Minute,
		DialTimeout:      10 * time.Second,
		Username:         cfg.Username,
		Password:         cfg.Password,
		Logger:           zap.NewNop(),
	})
	if err != nil {
		logger.Logger.Fatalf("etcd client starts failed: %v", err)
	}
	stopFunc, _ := taskutil.RunPeriodicalTask(10*time.Second, 10*time.Second, d.watch)
	quit.AddShutdownHook(func() {
		client.Close()
		stopFunc()
	})
	d.client = clientv3.NewKV(client)
	d.cache = make(map[string]lb.LoadBalancer)
	return d
}

type multiEtcdDiscovery struct {
	multiEtcd map[string]Discovery
	localZone string
}

func (m *multiEtcdDiscovery) Discover(ctx context.Context, name string) ([]lb.Server, error) {
	return m.DiscoverWithZone(ctx, m.localZone, name)
}

func (m *multiEtcdDiscovery) DiscoverWithZone(ctx context.Context, zone string, name string) ([]lb.Server, error) {
	d, b := m.multiEtcd[zone]
	if !b {
		return nil, lb.ServerNotFound
	}
	return d.Discover(ctx, name)
}

func (m *multiEtcdDiscovery) ChooseServer(ctx context.Context, name string) (lb.Server, error) {
	return m.ChooseServerWithZone(ctx, m.localZone, name)
}

func (m *multiEtcdDiscovery) ChooseServerWithZone(ctx context.Context, zone, name string) (lb.Server, error) {
	d, b := m.multiEtcd[zone]
	if !b {
		return lb.Server{}, lb.ServerNotFound
	}
	return d.ChooseServer(ctx, name)
}

func NewMultiEtcdDiscovery() Discovery {
	multi := static.GetMapSlice("discovery.multi")
	cfgList := make([]EtcdConfig, 0, len(multi))
	for _, cfg := range multi {
		cfgList = append(cfgList, EtcdConfig{
			Endpoints: cast.ToString(cfg["endpoints"]),
			Username:  cast.ToString(cfg["username"]),
			Password:  cast.ToString(cfg["password"]),
			Zone:      cast.ToString(cfg["zone"]),
		})
	}
	return newMultiEtcdDiscovery(cfgList)
}

func newMultiEtcdDiscovery(cfgList []EtcdConfig) Discovery {
	if len(cfgList) == 0 {
		logger.Logger.Fatalf("emtpy cfgList in MultiEtcdDiscovery")
	}
	localZone := static.GetString("discovery.zone")
	if localZone == "" {
		logger.Logger.Fatalf("empty discovery.zone")
	}
	multiEtcd := make(map[string]Discovery, 8)
	for _, etcdCfg := range cfgList {
		zone := etcdCfg.Zone
		if zone == "" {
			continue
		}
		_, b := multiEtcd[zone]
		if b {
			logger.Logger.Fatalf("duplicated zone")
		}
		multiEtcd[zone] = newEtcdDiscovery(etcdCfg)
	}
	return &multiEtcdDiscovery{
		multiEtcd: multiEtcd,
		localZone: localZone,
	}
}
