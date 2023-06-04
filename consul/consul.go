package consul

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/hashicorp/consul/api"
	"net/http"
)

func NewConsulClient(consulAddr, consulToken string) *api.Client {
	if consulAddr == "" {
		logger.Logger.Panic("empty consul addr")
	}
	if consulToken == "" {
		logger.Logger.Panic("empty consul token")
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
