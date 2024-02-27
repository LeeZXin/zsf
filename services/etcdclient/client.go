package etcdclient

import (
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

var (
	etcdUsername = static.GetString("etcd.username")
	etcdPassword = static.GetString("etcd.password")
	etcdHosts    = static.GetString("etcd.hosts")

	initOnce = sync.Once{}

	client *clientv3.Client
)

func NewKV() clientv3.KV {
	initEtcd()
	return clientv3.NewKV(client)
}

func GetClient() *clientv3.Client {
	initEtcd()
	return client
}

func initEtcd() {
	initOnce.Do(func() {
		var err error
		client, err = clientv3.New(clientv3.Config{
			Endpoints:        strings.Split(etcdHosts, ";"),
			AutoSyncInterval: time.Minute,
			DialTimeout:      10 * time.Second,
			Username:         etcdUsername,
			Password:         etcdPassword,
			Logger:           zap.NewNop(),
		})
		if err != nil {
			logger.Logger.Fatalf("etcd client starts failed: %v", err)
		}
		quit.AddShutdownHook(func() {
			client.Close()
		})
	})
}
