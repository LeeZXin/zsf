package discovery

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	memclient "github.com/LeeZXin/zsf/registry/mem/memclient"
)

var (
	memClient *memclient.RegistryClient
)

func init() {
	memClient = memclient.NewRegistryClient(static.GetString("memRegistry.host"), static.GetString("memRegistry.token"))
}

type MemDiscovery struct{}

func (*MemDiscovery) GetDiscoveryType() string {
	return MemDiscoveryType
}

func (s *MemDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	services, err := memClient.GetServiceInfoList(context.Background(), name)
	if err != nil {
		logger.Logger.Error(err)
		return nil, err
	}
	res := make([]ServiceAddr, 0, len(services))
	for _, service := range services {
		address := s.convert2ServiceAddr(service)
		res = append(res, address)
	}
	return res, err
}

// convert2ServiceAddr 转化为ServiceAddress
func (*MemDiscovery) convert2ServiceAddr(service memclient.ServiceInfoDTO) ServiceAddr {
	return ServiceAddr{
		Addr:    service.Ip,
		Port:    service.Port,
		Weight:  service.Weight,
		Version: service.Version,
	}
}
