package registry

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/hashicorp/consul/api"
	"sync"
	"time"
)

// consul路由注册实现
// 写代码时是用consul v1.5.2版本测试

var (
	consulClient *api.Client
)

func init() {
	consulClient = consul.NewConsulClient(static.GetString("consul.address"), static.GetString("consul.token"))
}

type consulImpl struct {
	serviceId      string
	checkID        string
	cancelFunc     context.CancelFunc
	ctx            context.Context
	info           ServiceInfo
	rpcName        string
	deregisterOnce sync.Once
}

func newConsulImpl(info ServiceInfo) *consulImpl {
	serviceId := fmt.Sprintf("service-%s.%s-%s", common.GetRegion(), common.GetZone(), idutil.RandomUuid())
	checkID := serviceId + "-checkID"
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &consulImpl{
		info:       info,
		ctx:        ctx,
		cancelFunc: cancelFunc,
		serviceId:  serviceId,
		checkID:    checkID,
	}
}

func (s *consulImpl) RegisterSelf() {
	// 向consul注册自己
	go func() {
		agent := consulClient.Agent()
		for {
			if s.ctx.Err() != nil {
				return
			}
			//重试注册
			err2 := agent.ServiceRegister(&api.AgentServiceRegistration{
				ID:   s.serviceId,
				Name: s.info.GetRpcName(),
				Tags: []string{
					common.ProtocolPrefix + s.info.Scheme,
					common.VersionPrefix + cmd.GetVersion(),
				},
				Port:    s.info.Port,
				Address: common.GetLocalIP(),
				Weights: &api.AgentWeights{
					Passing: s.info.Weight,
				},
				Check: &api.AgentServiceCheck{
					CheckID:                        s.checkID,
					TTL:                            "10s",
					DeregisterCriticalServiceAfter: "1m",
					Timeout:                        "5s",
					TLSSkipVerify:                  true,
				},
				EnableTagOverride: true,
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
					err = agent.PassTTL(s.checkID, "")
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

func (s *consulImpl) DeregisterSelf() {
	s.deregisterOnce.Do(func() {
		//取消注册
		s.cancelFunc()
		//服务关闭时注销自己
		err := consulClient.Agent().ServiceDeregister(s.serviceId)
		logger.Logger.Info("deregister serviceId:", s.serviceId)
		if err != nil {
			logger.Logger.Error(err)
		}
	})
}

type consulRegistry struct {
}

func (s *consulRegistry) GetRegistryType() string {
	return ConsulRegistryType
}

func (s *consulRegistry) RegisterSelf(info ServiceInfo) DeregisterAction {
	impl := newConsulImpl(info)
	impl.RegisterSelf()
	return impl
}
