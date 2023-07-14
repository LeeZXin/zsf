package discovery

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/sa_registry/client"
)

var (
	saClient *client.RegistryClient
)

func init() {
	saClient = client.NewRegistryClient(property.GetString("saRegistry.host"), property.GetString("saRegistry.token"))
}

type SaDiscovery struct{}

func (*SaDiscovery) GetDiscoveryType() string {
	return SaDiscoveryType
}

func (s *SaDiscovery) GetServiceInfo(name string) ([]ServiceAddr, error) {
	services, err := saClient.GetServiceInfoList(context.Background(), name)
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
func (*SaDiscovery) convert2ServiceAddr(service client.ServiceInfoDTO) ServiceAddr {
	return ServiceAddr{
		Addr:    service.Ip,
		Port:    service.Port,
		Weight:  service.Weight,
		Version: service.Version,
	}
}
