package discovery

import (
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/hashicorp/consul/api"
)

type ConsulDiscovery struct {
	consulClient *api.Client
}

func NewConsulDiscovery(consulClient *api.Client) IDiscovery {
	if consulClient == nil {
		consulClient = consul.NewConsulClient(
			static.GetString("discovery.consul.address"),
			static.GetString("discovery.consul.token"),
		)
	}
	return &ConsulDiscovery{consulClient: consulClient}
}

func (*ConsulDiscovery) GetDiscoveryType() string {
	return ConsulDiscoveryType
}

func (c *ConsulDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	services, _, err := c.consulClient.Health().Service(name, "", true, nil)
	if err != nil {
		logger.Logger.Error(err)
		return nil, err
	}
	res := make([]ServiceAddr, 0, len(services))
	for _, service := range services {
		address := c.convert2ServiceAddr(service)
		res = append(res, address)
	}
	return res, err
}

// convert2ServiceAddr 转化为ServiceAddress
func (*ConsulDiscovery) convert2ServiceAddr(service *api.ServiceEntry) ServiceAddr {
	return ServiceAddr{
		Addr:    service.Service.Address,
		Port:    service.Service.Port,
		Weight:  service.Service.Weights.Passing,
		Version: findServiceTagVersion(service.Service.Tags),
	}
}
