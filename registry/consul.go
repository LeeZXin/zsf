package registry

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"strings"
	"time"
)

// consul路由注册实现
// 写代码时是用consul v1.5.2版本测试

type consulImpl struct {
	serviceId string
	checkID   string

	cancelFunc context.CancelFunc
	ctx        context.Context

	info ServiceInfo
}

func (s *consulImpl) StartRegisterSelf() error {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	info := s.info
	agent := consul.GetConsulClient().Agent()
	s.serviceId = fmt.Sprintf("service-%s.%s-%s", appinfo.GetRegion(), appinfo.GetZone(),
		strings.ReplaceAll(uuid.New().String(), "-", ""))
	s.checkID = s.serviceId + "-checkID"

	quit.AddShutdownHook(func() {
		//取消注册
		s.cancelFunc()
		//服务关闭时注销自己
		err := agent.ServiceDeregister(s.serviceId)
		logger.Logger.Info("deregister serviceId:", s.serviceId)
		if err != nil {
			logger.Logger.Error(err)
		}
	})

	// 向consul注册自己
	go func() {
		registerFunc := func() error {
			return agent.ServiceRegister(&api.AgentServiceRegistration{
				ID:   s.serviceId,
				Name: appinfo.GetApplicationName() + "-" + info.Scheme,
				Tags: []string{
					info.Scheme,
					appinfo.VersionPrefix + cmd.GetVersion(),
				},
				Port:    info.Port,
				Address: appinfo.GetLocalIp(),
				Weights: &api.AgentWeights{
					Passing: info.Weight,
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
	return nil
}

type ConsulRegistry struct{}

func (s *ConsulRegistry) GetRegistryType() string {
	return ConsulRegistryType
}

func (s *ConsulRegistry) StartRegisterSelf(info ServiceInfo) error {
	impl := consulImpl{info: info}
	return impl.StartRegisterSelf()
}
