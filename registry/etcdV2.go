package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/quit"
	"go.etcd.io/etcd/client/v2"
	"time"
)

const (
	servicePrefix = "/rpc/"
)

// EtcdV2Registry etcd注册服务
type EtcdV2Registry struct {
	etcdClient client.KeysAPI
	serviceId  string
}

func NewEtcdV2Registry(endPoints []string, username, password string) IRegistry {
	c, err := client.New(client.Config{
		Endpoints: endPoints,
		Username:  username,
		Password:  password,
	})
	if err != nil {
		return &ErrorRegistry{
			registryType: EtcdV2RegistryType,
			err:          err,
		}
	}
	return &EtcdV2Registry{
		etcdClient: client.NewKeysAPI(c),
	}
}

type ServiceAddr struct {
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Version string `json:"version"`
}

func (e *EtcdV2Registry) GetRegistryType() string {
	return EtcdV2RegistryType
}

func (e *EtcdV2Registry) StartRegisterSelf(info ServiceInfo) error {
	rpcName := fmt.Sprintf("%s-%s/%s", common.GetApplicationName(), info.Scheme, common.GetInstanceId())
	addrInfoJson, err := json.Marshal(ServiceAddr{
		Addr:    common.GetLocalIP(),
		Port:    info.Port,
		Weight:  info.Weight,
		Version: cmd.GetVersion(),
	})
	if err != nil {
		return err
	}
	task, _ := taskutil.NewPeriodicalTask(8*time.Second, func() {
		_, _ = e.etcdClient.Set(context.Background(), servicePrefix+rpcName, string(addrInfoJson), &client.SetOptions{
			Dir:     false,
			TTL:     10 * time.Second,
			Refresh: true,
		})
	})
	task.Start()
	quit.AddShutdownHook(func() {
		task.Stop()
		// 注销自己
		_, _ = e.etcdClient.Delete(context.Background(), servicePrefix+rpcName, nil)
	})
	return nil
}
