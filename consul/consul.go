package consul

import (
	"github.com/hashicorp/consul/api"
	"net/http"
	"zsf/logger"
	"zsf/property"
)

// 全局唯一consul client

var (
	Client *api.Client
)

func init() {
	// 当四者其中一种有需要时，初始化consul
	if property.GetBool("http.registry.enabled") ||
		property.GetBool("grpc.registry.enabled") ||
		property.GetBool("property.enabled") ||
		property.GetBool("discovery.enabled") {
		consulAddr := property.GetString("consul.address")
		if consulAddr == "" {
			logger.Logger.Panic("consul.address is empty")
		}
		consulToken := property.GetString("consul.token")
		if consulToken == "" {
			logger.Logger.Panic("consul.token is empty")
		}
		defaultConfig := api.DefaultConfig()
		defaultConfig.Address = consulAddr
		var err error
		Client, err = api.NewClient(defaultConfig)
		if err != nil {
			logger.Logger.Panic(err)
		}
		Client.SetHeaders(http.Header{
			"X-Consul-Token": []string{consulToken},
		})
	}
}
