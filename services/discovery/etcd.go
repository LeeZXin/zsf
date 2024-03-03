package discovery

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/localcache"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/services/lb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"strings"
	"time"
)

type etcdDiscovery struct {
	client clientv3.KV
	router localcache.ExpireCache[lb.LoadBalancer]
}

func (*etcdDiscovery) GetDiscoveryType() string {
	return EtcdDiscoveryType
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
	balancer, err := d.router.LoadData(ctx, name)
	if err != nil {
		return lb.Server{}, err
	}
	return balancer.ChooseServer(ctx)
}

func newEtcdDiscovery(endpoints, username, password string) *etcdDiscovery {
	ret := new(etcdDiscovery)
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
	ret.client = clientv3.NewKV(client)
	ret.router, _ = localcache.NewLocalCache(func(ctx context.Context, key string) (lb.LoadBalancer, error) {
		servers, err := ret.Discover(key)
		if err != nil {
			logger.Logger.WithContext(ctx).Error(err)
			return nil, err
		}
		balancer := &lb.NearbyLoadBalancer{
			LbPolicy: lb.Policy(lbPolicy),
		}
		balancer.SetServers(servers)
		return balancer, nil
	}, 10*time.Second)
	return ret
}
