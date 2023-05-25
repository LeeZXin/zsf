package loader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/executor"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/psub"
	"github.com/LeeZXin/zsf/quit"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"
	"sync"
	"time"
)

var (
	registerMu    = sync.Mutex{}
	watchKeys     = make(map[string]bool)
	notifyChannel *psub.Channel
)

func init() {
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, &executor.CallerRunsPolicy{})
	notifyChannel, _ = psub.NewChannel(channelExecutor)
}

type ChangeCallback func()

func startWatchPropertyChange() {
	propertyKey := fmt.Sprintf("%s/property/www/%s", cmd.GetEnv(), common.GetApplicationName())
	logger.Logger.Info("listen consul property key:", propertyKey)

	plan, err := watch.Parse(map[string]any{
		"type": "key",
		"key":  propertyKey,
	})
	if err != nil {
		logger.Logger.Panic(err)
	}

	var firstModifyIndex uint64
	//首次需要加载远程配置
	kv, _, err := consul.GetConsulClient().KV().Get(propertyKey, nil)
	if err == nil {
		err = property.MergeConfig(bytes.NewReader(kv.Value))
		if err != nil {
			logger.Logger.Error(err)
		} else {
			firstModifyIndex = kv.ModifyIndex
		}
	} else {
		logger.Logger.Error(err)
	}
	plan.Handler = func(u uint64, i interface{}) {
		//防止触发两次
		if u == firstModifyIndex {
			return
		}
		if i == nil {
			return
		}
		kvPair := i.(*api.KVPair)
		if kvPair.Value != nil {
			listenKeys := make([]string, 0)
			registerMu.Lock()
			if len(watchKeys) > 0 {
				for watchKey := range watchKeys {
					listenKeys = append(listenKeys, watchKey)
				}
			}
			registerMu.Unlock()
			//获取旧配置
			oldProperties := getAllProperties(listenKeys)
			//合并配置
			err = property.MergeConfig(bytes.NewReader(kvPair.Value))
			if err != nil {
				logger.Logger.Error(err)
				return
			}
			//获取新配置
			newProperties := getAllProperties(listenKeys)
			//检查不同
			for _, key := range listenKeys {
				if oldProperties[key] != newProperties[key] {
					//发送广播
					_ = notifyChannel.Publish(key, "")
				}
			}
		}
	}
	go func() {
		running := true
		quit.AddShutdownHook(func() {
			running = false
			plan.Stop()
		})
		//持续监听
		for running {
			listenErr := plan.RunWithClientAndHclog(consul.GetConsulClient(), hclog.NewNullLogger())
			if listenErr != nil {
				logger.Logger.Error(listenErr)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

// getAllProperties 获取配置
func getAllProperties(listenKeys []string) map[string]string {
	properties := make(map[string]string, len(listenKeys))
	for _, key := range listenKeys {
		oldProperty := property.Get(key)
		if oldProperty == nil {
			properties[key] = ""
		} else {
			jsonContent, err := json.Marshal(oldProperty)
			if err == nil {
				properties[key] = string(jsonContent)
			} else {
				properties[key] = ""
			}
		}
	}
	return properties
}

func init() {
	enabled := property.GetBool("property.enabled")
	if enabled {
		//启动consul配置监听
		startWatchPropertyChange()
	}
}

// OnKeyChange 监听某个key的变化来触发回调
func OnKeyChange(key string, callback ChangeCallback) {
	if key == "" || callback == nil {
		return
	}
	logger.Logger.Info("listen property key change:", key)
	registerMu.Lock()
	defer registerMu.Unlock()
	watchKeys[key] = true
	_ = notifyChannel.Subscribe(key, func(data any) {
		callback()
	})
}
