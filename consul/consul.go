package consul

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/hashicorp/consul/api"
	"net/http"
)

func NewConsulClient(consulAddr, consulToken string) *api.Client {
	if consulAddr == "" {
		consulAddr = "127.0.0.1:8500"
	}
	defaultConfig := api.DefaultConfig()
	defaultConfig.Address = consulAddr
	client, err := api.NewClient(defaultConfig)
	if err != nil {
		logger.Logger.Panic(err)
	}
	client.SetHeaders(http.Header{
		"X-Consul-Token": []string{consulToken},
	})
	return client
}
