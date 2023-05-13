package property_loader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/go-hclog"
	"sync"
	"time"
	"zsf/app"
	"zsf/common"
	"zsf/consul"
	"zsf/executor"
	"zsf/logger"
	"zsf/property"
	"zsf/psub"
	"zsf/quit"
)

var (
	registerChangeCallbackMu = sync.Mutex{}
	watchKeys                = make(map[string]bool)
	notifyChannel            *psub.Channel
)

func init() {
	e, _ := executor.NewExecutor(2, 8, time.Minute, &executor.CallerRunsPolicy{})
	notifyChannel, _ = psub.NewChannel(e)
}

type PropertyChange func()

func startWatchPropertyChange() {
	key := fmt.Sprintf("%s/property/www/%s", common.Env, app.ApplicationName)
	logger.Logger.Info("listen consul property key:", key)
	plan, err := watch.Parse(map[string]any{
		"type": "key",
		"key":  key,
	})
	if err != nil {
		logger.Logger.Panic(err)
	}
	var firstModifyIndex uint64
	//首次需要加载远程配置
	kv, _, err := consul.Client.KV().Get(key, nil)
	if err == nil {
		err2 := property.MergeConfig(bytes.NewReader(kv.Value))
		if err2 != nil {
			logger.Logger.Error(err2)
		} else {
			firstModifyIndex = kv.ModifyIndex
		}
	} else {
		logger.Logger.Error(err)
	}
	plan.Handler = func(u uint64, i interface{}) {
		if u == firstModifyIndex {
			return
		}
		logger.Logger.Info("property from consul changed")
		if i == nil {
			return
		}
		m := i.(*api.KVPair)
		if m.Value != nil {
			//copy keys
			copyKeys := make(map[string]bool)
			registerChangeCallbackMu.Lock()
			if len(watchKeys) > 0 {
				for watchKey := range watchKeys {
					copyKeys[watchKey] = true
				}
			}
			registerChangeCallbackMu.Unlock()
			//保存旧配置
			oldProperties := getAll(copyKeys)
			err2 := property.MergeConfig(bytes.NewReader(m.Value))
			if err2 != nil {
				logger.Logger.Error(err2)
			}
			newProperties := getAll(copyKeys)
			//检查不同
			for k := range copyKeys {
				if oldProperties[k] != newProperties[k] {
					//发送广播
					_ = notifyChannel.Publish(k, "")
				}
			}
		}
	}
	go func() {
		running := true
		quit.RegisterQuitFunc(func() {
			running = false
			plan.Stop()
		})
		for running {
			err2 := plan.RunWithClientAndHclog(consul.Client, hclog.NewNullLogger())
			if err2 != nil {
				logger.Logger.Error(err2)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func getAll(keys map[string]bool) map[string]string {
	m := make(map[string]string)
	for k := range keys {
		p := property.Get(k)
		if p == nil {
			m[k] = ""
		} else {
			ml, e := json.Marshal(p)
			if e == nil {
				m[k] = string(ml)
			} else {
				m[k] = ""
			}
		}
	}
	return m
}

func init() {
	enabled := property.GetBool("property.enabled")
	if enabled {
		//启动consul配置中心
		startWatchPropertyChange()
	}
}

// RegisterKeyChangeWatcher 监听某个key的变化来触发回调
func RegisterKeyChangeWatcher(key string, change PropertyChange) {
	if key == "" || change == nil {
		return
	}
	registerChangeCallbackMu.Lock()
	defer registerChangeCallbackMu.Unlock()
	watchKeys[key] = true
	_ = notifyChannel.Subscribe(key, func(data any) {
		change()
	})
}
