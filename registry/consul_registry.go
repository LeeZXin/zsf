package registry

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/app"
	"github.com/LeeZXin/zsf/common"
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

type ConsulRegistry struct {
	Config     ServiceRegistryConfig
	serviceId  string
	checkID    string
	cancelFunc context.CancelFunc
	ctx        context.Context
}

func (s *ConsulRegistry) StartRegisterSelf() {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	config := s.Config
	agent := consul.Client.Agent()
	s.serviceId = fmt.Sprintf("service-%s.%s-%s", app.Region, app.Zone,
		strings.ReplaceAll(uuid.New().String(), "-", ""))
	s.checkID = s.serviceId + "-checkID"
	quit.RegisterQuitFunc(func() {
		s.cancelFunc()
		//服务关闭时注销自己
		err := agent.ServiceDeregister(s.serviceId)
		logger.Logger.Info("deregister serviceId:", s.serviceId)
		if err != nil {
			logger.Logger.Error(err)
		}
	})
	// 注册自己
	go func() {
		registerFunc := func() error {
			return agent.ServiceRegister(&api.AgentServiceRegistration{
				ID:      s.serviceId,
				Name:    config.ApplicationName + "-" + config.Scheme,
				Tags:    []string{s.Config.Scheme, common.VersionPrefix + common.Version},
				Port:    config.Port,
				Address: config.Ip,
				Weights: &api.AgentWeights{
					Passing: config.Weight,
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
						err = agent.PassTTL(s.checkID, "")
						break
					}
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
