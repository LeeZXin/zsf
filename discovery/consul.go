package discovery

import (
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
)

type ConsulDiscovery struct{}

func (*ConsulDiscovery) GetDiscoveryType() string {
	return ConsulDiscoveryType
}

func (*ConsulDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	services, _, err := consul.GetConsulClient().Health().Service(name, "", true, nil)
	if err != nil {
		logger.Logger.Error(err)
		return nil, err
	}
	res := make([]ServiceAddr, 0, len(services))
	for _, service := range services {
		address := convert2ServiceAddr(service)
		res = append(res, address)
	}
	return res, err
}
