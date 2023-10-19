package discovery

import (
	"context"
	"encoding/json"
	"go.etcd.io/etcd/client/v2"
	"time"
)

const (
	servicePrefix = "/rpc/"
)

// EtcdV2Discovery etcd服务发现
type EtcdV2Discovery struct {
	etcdClient client.KeysAPI
}

func NewEtcdV2Discovery(endPoints []string, username, password string) IDiscovery {
	c, err := client.New(client.Config{
		Endpoints: endPoints,
		Username:  username,
		Password:  password,
	})
	if err != nil {
		return &ErrorDiscovery{
			discoveryType: EtcdV2DiscoveryType,
			err:           err,
		}
	}
	return &EtcdV2Discovery{
		etcdClient: client.NewKeysAPI(c),
	}
}

func (e *EtcdV2Discovery) GetDiscoveryType() string {
	return EtcdV2DiscoveryType
}

func (e *EtcdV2Discovery) GetServiceInfo(target string) ([]ServiceAddr, error) {
	timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := e.etcdClient.Get(timeout, servicePrefix+target, &client.GetOptions{
		Recursive: true,
	})
	if err != nil {
		return nil, err
	}
	kvs := res.Node.Nodes
	ret := make([]ServiceAddr, len(kvs))
	for _, value := range kvs {
		var ep ServiceAddr
		if value.Value != "" {
			err = json.Unmarshal([]byte(value.Value), &ep)
			if err == nil {
				ret = append(ret, ep)
			}
		} else if value.Nodes.Len() > 0 {
			for _, n := range value.Nodes {
				err = json.Unmarshal([]byte(n.Value), &ep)
				if err == nil {
					ret = append(ret, ep)
				}
			}
		}
	}
	return ret, nil
}
