package dynamic

import (
	"bytes"
	"fmt"
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/hashset"
	"github.com/LeeZXin/zsf-utils/psub"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/hashicorp/consul/api"
	"sync"
	"time"
)

const (
	ConsulType = "consul"
)

var (
	consulImpl = newConsulProperty()
)

type consulProperty struct {
	registerMu    sync.Mutex
	watchKeys     hashset.Set[string]
	notifyChannel *psub.Channel[any]

	fetchTask *taskutil.PeriodicalTask

	propertyKey string
	client      *api.Client
}

func newConsulProperty() *consulProperty {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, executor.CallerRunsStrategy)
	notifyChannel, _ := psub.NewChannel[any](channelExecutor)
	ret := &consulProperty{
		notifyChannel: notifyChannel,
		watchKeys:     hashset.NewConcurrentHashSet[string](nil),
		propertyKey:   fmt.Sprintf("%s/property/www/%s", cmd.GetEnv(), static.GetString("application.name")),
		client:        consul.NewConsulClient(static.GetString("property.consul.address"), static.GetString("property.consul.token")),
	}
	task, _ := taskutil.NewPeriodicalTask(30*time.Second, ret.loadProperty)
	task.Start()
	ret.fetchTask = task
	return ret
}

func (p *consulProperty) GetPropertyType() string {
	return ConsulType
}

func (p *consulProperty) OnKeyChange(k string, c KeyChangeCallback) {
	if k == "" || c == nil {
		return
	}
	p.watchKeys.Add(k)
	_ = p.notifyChannel.Subscribe(k, func(data any) {
		c()
	})
}

func (p *consulProperty) OnApplicationStart() {
	//首次需要加载远程配置
	kv, _, err := p.client.KV().Get(p.propertyKey, nil)
	err = MergeConfig(bytes.NewReader(kv.Value))
	if err != nil {
		logger.Logger.Error(err.Error())
		return
	}
}

func (p *consulProperty) loadProperty() {
	kv, _, err := p.client.KV().Get(p.propertyKey, nil)
	if err == nil {
		//获取旧配置
		keys := p.watchKeys.AllKeys()
		oldProperties := getAllProperties(keys)
		err = MergeConfig(bytes.NewReader(kv.Value))
		if err != nil {
			logger.Logger.Error(err.Error())
			return
		}
		if err != nil {
			return
		}
		//获取新配置
		newProperties := getAllProperties(keys)
		//寻找不同
		for _, key := range keys {
			if oldProperties[key] != newProperties[key] {
				//发送广播
				_ = p.notifyChannel.Publish(key, "")
			}
		}
	}
}

func (p *consulProperty) AfterInitialize() {
}

func (p *consulProperty) OnApplicationShutdown() {
	p.fetchTask.Stop()
}
