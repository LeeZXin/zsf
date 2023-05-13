package consul

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/hashicorp/consul/api"
	"net/http"
	"sync"
)

// 全局唯一consul client

var (
	client *api.Client

	mu = sync.Mutex{}
)

func GetConsulClient() *api.Client {
	if client == nil {
		mu.Lock()
		if client == nil {
			initClient()
		}
		mu.Unlock()
	}
	return client
}

func initClient() {
	consulAddr := property.GetString("consul.address")
	if consulAddr == "" {
		logger.Logger.Panic("empty consul addr")
	}

	consulToken := property.GetString("consul.token")
	if consulToken == "" {
		logger.Logger.Panic("empty consul token")
	}

	defaultConfig := api.DefaultConfig()
	defaultConfig.Address = consulAddr

	var err error
	client, err = api.NewClient(defaultConfig)
	if err != nil {
		logger.Logger.Panic(err)
	}
	client.SetHeaders(http.Header{
		"X-Consul-Token": []string{consulToken},
	})
}
