package registry

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
	"strings"
	"time"
)

//服务发现
//目前只实现consul

// ServiceInfo 注册所需的信息
type ServiceInfo struct {
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight int
}

func RegisterSelf(info ServiceInfo) {
	r := ConsulRegistry{
		Info: info,
	}
	r.StartRegisterSelf()
}

// consul路由注册实现
// 写代码时是用consul v1.5.2版本测试

type ConsulRegistry struct {
	Info ServiceInfo

	serviceId string
	checkID   string

	cancelFunc context.CancelFunc
	ctx        context.Context
}

func (s *ConsulRegistry) StartRegisterSelf() {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	info := s.Info

	agent := consul.GetConsulClient().Agent()
	s.serviceId = fmt.Sprintf("service-%s.%s-%s", appinfo.Region, appinfo.Zone,
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
				ID:      s.serviceId,
				Name:    appinfo.ApplicationName + "-" + info.Scheme,
				Tags:    []string{s.Info.Scheme, appinfo.VersionPrefix + appinfo.Version},
				Port:    info.Port,
				Address: appinfo.LocalIP,
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
}
