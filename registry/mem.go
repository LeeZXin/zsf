package registry

import (
	"context"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/registry/mem/memclient"
	"time"
)

var (
	saClient *memclient.RegistryClient
)

func init() {
	saClient = memclient.NewRegistryClient(static.GetString("saRegistry.host"), static.GetString("saRegistry.token"))
}

type memImpl struct {
	instanceId  string
	serviceName string

	cancelFunc context.CancelFunc
	ctx        context.Context

	info ServiceInfo
}

func (s *memImpl) StartRegisterSelf() {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	info := s.info
	s.instanceId = common.GetInstanceId()
	s.serviceName = common.GetApplicationName() + "-" + info.Scheme
	// 注册自己
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				break
			}
			//重试注册
			err2 := saClient.RegisterService(context.Background(), memclient.RegisterServiceReqDTO{
				ServiceName:   s.serviceName,
				Ip:            common.GetLocalIP(),
				Port:          info.Port,
				InstanceId:    s.instanceId,
				Weight:        info.Weight,
				Version:       cmd.GetVersion(),
				LeaseDuration: 20,
			})
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
					err = saClient.PassTTL(context.Background(), memclient.PassTtlReqDTO{
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
}

func (s *memImpl) DeregisterSelf() {
	//取消注册
	s.cancelFunc()
	//服务关闭时注销自己
	err := saClient.DeregisterService(context.Background(), memclient.DeregisterServiceReqDTO{
		ServiceName: s.serviceName,
		InstanceId:  s.instanceId,
	})
	logger.Logger.Info("deregister serviceId:", s.serviceName)
	if err != nil {
		logger.Logger.Error(err)
	}
}

type MemRegistry struct{}

func (s *MemRegistry) GetRegistryType() string {
	return MemRegistryType
}

func (s *MemRegistry) StartRegisterSelf(info ServiceInfo) IDeregister {
	impl := &memImpl{info: info}
	impl.StartRegisterSelf()
	return impl
}
