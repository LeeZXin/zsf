package registry

import (
	"context"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/sa_registry/client"
	"time"
)

var (
	saClient *client.RegistryClient
)

func init() {
	saClient = client.NewRegistryClient(property.GetString("saRegistry.host"), property.GetString("saRegistry.token"))
}

type saImpl struct {
	instanceId  string
	serviceName string

	cancelFunc context.CancelFunc
	ctx        context.Context

	info ServiceInfo
}

func (s *saImpl) StartRegisterSelf() error {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	info := s.info
	s.instanceId = common.GetInstanceId()
	s.serviceName = common.GetApplicationName() + "-" + info.Scheme
	quit.AddShutdownHook(func() {
		//取消注册
		s.cancelFunc()
		//服务关闭时注销自己
		err := saClient.DeregisterService(context.Background(), client.DeregisterServiceReqDTO{
			ServiceName: s.serviceName,
			InstanceId:  s.instanceId,
		})
		logger.Logger.Info("deregister serviceId:", s.serviceName)
		if err != nil {
			logger.Logger.Error(err)
		}
	})

	// 向consul注册自己
	go func() {
		registerFunc := func() error {
			return saClient.RegisterService(context.Background(), client.RegisterServiceReqDTO{
				ServiceName:   s.serviceName,
				Ip:            common.GetLocalIp(),
				Port:          info.Port,
				InstanceId:    s.instanceId,
				Weight:        info.Weight,
				Version:       cmd.GetVersion(),
				LeaseDuration: 20,
			})
		}
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				break
			}
			//重试注册
			err2 := registerFunc()
			if err2 == nil {
				ticker := time.NewTicker(8 * time.Second)
				for {
					var err error
					select {
					case <-s.ctx.Done():
						ticker.Stop()
						return
					case <-ticker.C:
						break
					}
					err = saClient.PassTTL(context.Background(), client.PassTtlReqDTO{
						ServiceName: s.serviceName,
						InstanceId:  s.instanceId,
					})
					if err != nil {
						logger.Logger.Error(err)
						ticker.Stop()
						break
					}
				}
			} else {
				logger.Logger.Error(err2)
			}
			time.Sleep(10 * time.Second)
		}
	}()
	return nil
}

type SaRegistry struct{}

func (s *SaRegistry) GetRegistryType() string {
	return SaRegistryType
}

func (s *SaRegistry) StartRegisterSelf(info ServiceInfo) error {
	impl := saImpl{info: info}
	return impl.StartRegisterSelf()
}
