package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/psub"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/consul"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/quit"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/viper"
	"io"
	"sync"
	"time"
)

var (
	v             *viper.Viper
	registerMu    = sync.Mutex{}
	watchKeys     = make(map[string]bool)
	notifyChannel *psub.Channel[any]

	ctx      context.Context
	cancelFn context.CancelFunc
)

func init() {
	ctx, cancelFn = context.WithCancel(context.Background())
	v = viper.New()
	v.SetConfigType("yaml")
	channelExecutor, _ := executor.NewExecutor(2, 8, time.Minute, executor.CallerRunsStrategy)
	notifyChannel, _ = psub.NewChannel[any](channelExecutor)
	//启动consul配置监听
	startWatchPropertyChange()
}

type ChangeCallback func()

func startWatchPropertyChange() {
	propertyKey := fmt.Sprintf("%s/property/www/%s", cmd.GetEnv(), static.GetString("application.name"))
	plan, err := watch.Parse(map[string]any{
		"type": "key",
		"key":  propertyKey,
	})
	if err != nil {
		panic(err)
	}
	consulClient := consul.NewConsulClient(static.GetString("property.consul.address"), static.GetString("property.consul.token"))
	var firstModifyIndex uint64
	//首次需要加载远程配置
	kv, _, err := consulClient.KV().Get(propertyKey, nil)
	if err == nil {
		err = v.MergeConfig(bytes.NewReader(kv.Value))
		if err == nil {
			firstModifyIndex = kv.ModifyIndex
		} else {
			logger.Logger.Error(err.Error())
		}
	}
	plan.Handler = func(u uint64, i any) {
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
			err = MergeConfig(bytes.NewReader(kvPair.Value))
			if err != nil {
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
		quit.AddShutdownHook(func() {
			cancelFn()
			plan.Stop()
		})
		//持续监听
		for {
			if ctx.Err() != nil {
				return
			}
			listenErr := plan.RunWithClientAndHclog(consulClient, hclog.NewNullLogger())
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
		oldProperty := static.Get(key)
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

// OnKeyChange 监听某个key的变化来触发回调
func OnKeyChange(key string, callback ChangeCallback) {
	if key == "" || callback == nil {
		return
	}
	registerMu.Lock()
	defer registerMu.Unlock()
	watchKeys[key] = true
	_ = notifyChannel.Subscribe(key, func(data any) {
		callback()
	})
}

func GetString(key string) string {
	return v.GetString(key)
}

func GetInt(key string) int {
	return v.GetInt(key)
}

func Get(key string) any {
	return v.Get(key)
}

func GetBool(key string) bool {
	return v.GetBool(key)
}

func GetFloat64(key string) float64 {
	return v.GetFloat64(key)
}

func GetStringMapString(key string) map[string]string {
	return v.GetStringMapString(key)
}

func GetStringMap(key string) map[string]any {
	return v.GetStringMap(key)
}

func MergeConfig(in io.Reader) error {
	return v.MergeConfig(in)
}

func Exists(key string) bool {
	return v.IsSet(key)
}

func GetInt64(key string) int64 {
	return v.GetInt64(key)
}
