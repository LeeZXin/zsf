package dynamic

import (
	"bytes"
	"fmt"
	"github.com/LeeZXin/zsf-utils/collections/hashset"
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/psub"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"
	"time"
)

const (
	ConsulType = "consul"
)

var (
	consulImpl = newConsulProperty()
)

type consulProperty struct {
	watchKeys        hashset.Set[string]
	notifyChannel    *psub.Channel[any]
	propertyKey      string
	client           *api.Client
	firstModifyIndex uint64
	plan             *watch.Plan
}

func newConsulProperty() *consulProperty {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, executor.CallerRunsStrategy)
	notifyChannel, _ := psub.NewChannel[any](channelExecutor)
	ret := &consulProperty{
		notifyChannel: notifyChannel,
		watchKeys:     hashset.NewConcurrentHashSet[string](),
		propertyKey:   fmt.Sprintf("%s/property/www/%s", cmd.GetEnv(), common.GetApplicationName()),
		client:        consul.NewConsulClient(static.GetString("property.consul.address"), static.GetString("property.consul.token")),
	}
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
	if err == nil {
		p.firstModifyIndex = kv.ModifyIndex
	}
	p.plan, err = watch.Parse(map[string]any{
		"type": "key",
		"key":  p.propertyKey,
	})
	if err != nil {
		logger.Logger.Panic(err.Error())
	}
	p.plan.Handler = func(u uint64, i any) {
		//防止触发两次
		if u == p.firstModifyIndex {
			return
		}
		if i == nil {
			return
		}
		kvPair, b := i.(*api.KVPair)
		if b {
			p.notify(kvPair)
		}
	}
	go func() {
		//持续监听
		for {
			if p.plan.IsStopped() {
				return
			}
			listenErr := p.plan.RunWithClientAndHclog(p.client, hclog.NewNullLogger())
			if listenErr != nil {
				logger.Logger.Error(listenErr.Error())
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func (p *consulProperty) notify(kvPair *api.KVPair) {
	//获取旧配置
	keys := p.watchKeys.AllKeys()
	oldProperties := getAllProperties(keys)
	err := MergeConfig(bytes.NewReader(kvPair.Value))
	if err != nil {
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

func (p *consulProperty) AfterInitialize() {
}

func (p *consulProperty) OnApplicationShutdown() {
	p.plan.Stop()
}
