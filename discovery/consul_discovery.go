package discovery

import (
	"github.com/hashicorp/consul/api"
	"strings"
	"zsf/common"
	"zsf/logger"
)

// consul服务发现

var (
	impl IDiscovery
)

type consulDiscovery struct {
	client *api.Client
}

func findVersionInTags(tags []string) string {
	ver := common.DefaultVersion
	for _, tag := range tags {
		if strings.HasPrefix(tag, common.VersionPrefix) {
			ver = strings.TrimPrefix(tag, common.VersionPrefix)
			break
		}
	}
	return ver
}

func (s *consulDiscovery) GetServiceInfo(name string) ([]ServiceAddress, error) {
	services, _, err := s.client.Health().Service(name, "", true, nil)
	if err != nil {
		logger.Logger.Error(err)
		return nil, err
	}
	res := make([]ServiceAddress, 0, 8)
	for _, service := range services {
		address := packHealthyServiceAddress(service)
		res = append(res, address)
	}
	return res, err
}

func packHealthyServiceAddress(service *api.ServiceEntry) ServiceAddress {
	return ServiceAddress{
		Address: service.Service.Address,
		Port:    service.Service.Port,
		Weight:  service.Service.Weights.Passing,
		Version: findVersionInTags(service.Service.Tags),
	}
}

func GetServiceInfo(name string) ([]ServiceAddress, error) {
	return impl.GetServiceInfo(name)
}
