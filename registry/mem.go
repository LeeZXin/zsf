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
	instanceId string
	rpcName    string
	cancelFunc context.CancelFunc
	ctx        context.Context
	info       ServiceInfo
}

func newMemImpl(info ServiceInfo) *memImpl {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &memImpl{
		info:       info,
		ctx:        ctx,
		cancelFunc: cancelFunc,
		instanceId: common.GetInstanceId(),
	}
}

func (s *memImpl) StartRegisterSelf() {
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
				ServiceName:   s.rpcName,
				Ip:            common.GetLocalIP(),
				Port:          s.info.Port,
				InstanceId:    s.instanceId,
				Weight:        s.info.Weight,
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
						ServiceName: s.rpcName,
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
		ServiceName: s.rpcName,
		InstanceId:  s.instanceId,
	})
	logger.Logger.Info("deregister serviceId:", s.rpcName)
	if err != nil {
		logger.Logger.Error(err)
	}
}

type memRegistry struct{}

func (s *memRegistry) GetRegistryType() string {
	return MemRegistryType
}

func (s *memRegistry) RegisterSelf(info ServiceInfo) DeregisterAction {
	impl := &memImpl{info: info}
	impl.StartRegisterSelf()
	return impl
}
