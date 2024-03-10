package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"
)

var (
	ob = newObserver()
)

type observer struct {
	sync.RWMutex
	cache      map[string]*viper.Viper
	client     *clientv3.Client
	key        string
	ctx        context.Context
	cancelFunc context.CancelFunc
	rev        int64
}

func (o *observer) Close() {
	logger.Logger.Infof("dynamic property observer closed")
	o.cancelFunc()
	o.client.Close()
	o.Lock()
	defer o.Unlock()
	o.cache = nil
}

func newObserver() *observer {
	o := new(observer)
	o.cache = make(map[string]*viper.Viper, 8)
	o.key = common.PropertyPrefix + common.GetApplicationName() + "/"
	var err error
	o.client, err = clientv3.New(clientv3.Config{
		Endpoints:        strings.Split(static.GetString("property.dynamic.etcd.hosts"), ";"),
		AutoSyncInterval: time.Minute,
		DialTimeout:      10 * time.Second,
		Username:         static.GetString("property.dynamic.etcd.username"),
		Password:         static.GetString("property.dynamic.etcd.password"),
		Logger:           zap.NewNop(),
	})
	if err != nil {
		logger.Logger.Fatalf("property etcd client starts failed: %v", err)
	}
	o.ctx, o.cancelFunc = context.WithCancel(context.Background())
	quit.AddShutdownHook(o.Close)
	logger.Logger.Infof("start listening dynamic property key: %s", o.key)
	o.init()
	return o
}

type propObj struct {
	Key     string
	Content []byte
}

func (o *observer) readRemote() ([]propObj, int64) {
	response, err := o.client.Get(o.ctx, o.key, clientv3.WithPrefix())
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			logger.Logger.Fatalf("dynamic property permission denied")
		}
		logger.Logger.Error(err)
		return nil, 0
	}
	ret := make([]propObj, 0, len(response.Kvs))
	for _, kv := range response.Kvs {
		ret = append(ret, propObj{
			Key:     strings.TrimPrefix(string(kv.Key), o.key),
			Content: kv.Value,
		})
	}
	return ret, response.Header.GetRevision()
}

func (o *observer) watchRemote() {
	for {
		if o.ctx.Err() != nil {
			return
		}
		logger.Logger.Infof("try to watch prefix: %s with revision: %d", o.key, o.rev+1)
		watcher := clientv3.NewWatcher(o.client)
		wchan := watcher.Watch(o.ctx, o.key, clientv3.WithPrefix(), clientv3.WithRev(o.rev+1))
		o.dealChan(wchan)
		watcher.Close()
		time.Sleep(10 * time.Second)
	}
}

func (o *observer) deleteKey(key string) {
	o.Lock()
	defer o.Unlock()
	delete(o.cache, key)
}

func (o *observer) putKey(key string, v *viper.Viper) {
	o.Lock()
	defer o.Unlock()
	o.cache[key] = v
}

func (o *observer) getViper(key string) (*viper.Viper, bool) {
	o.RLock()
	defer o.RUnlock()
	ret, b := o.cache[key]
	return ret, b
}

func (o *observer) dealChan(wchan clientv3.WatchChan) {
	for {
		select {
		case <-o.ctx.Done():
			return
		case data, ok := <-wchan:
			if !ok || data.Canceled {
				logger.Logger.Info("dynamic property is canceled")
				if err := data.Err(); err != nil {
					logger.Logger.Error(err)
				}
				return
			}
			o.rev = data.Header.Revision
			for _, event := range data.Events {
				switch event.Type {
				case clientv3.EventTypeDelete:
					if event.Kv != nil {
						key := strings.TrimPrefix(string(event.Kv.Key), o.key)
						logger.Logger.Infof("delete dynamic key: %s", key)
						o.deleteKey(key)
					}
				case clientv3.EventTypePut:
					if event.Kv != nil {
						key := strings.TrimPrefix(string(event.Kv.Key), o.key)
						v, b, err := o.newViper(key, event.Kv.Value)
						if err == nil {
							logger.Logger.Infof("reset dynamic key: %s revision: %v", key, o.rev)
							if !b {
								o.putKey(key, v)
							}
						}
					}
				}
			}
		}
	}
}

func ext(name string) string {
	ret := path.Ext(name)
	if len(ret) > 0 {
		return ret[1:]
	}
	return ret
}

func (o *observer) newViper(key string, content []byte) (*viper.Viper, bool, error) {
	v, b := o.getViper(key)
	if !b {
		v = viper.New()
		v.SetConfigType(ext(key))
	}
	var val contentVal
	err := json.Unmarshal(content, &val)
	if err != nil {
		logger.Logger.Errorf("read remote config is not json format: %s", key)
		return nil, false, err
	}
	if val.Version == "" {
		logger.Logger.Errorf("read remote config version is empty: %s", key)
		return nil, false, errors.New("empty version")
	}
	err = v.MergeConfig(strings.NewReader(val.Content))
	if err != nil {
		logger.Logger.Errorf("merge remote config err, key: %s, err: %v", key, err)
		return nil, false, err
	}
	logger.Logger.Infof("merge remote config successfully key: %s, version: %s", key, val.Version)
	return v, b, nil
}

func (o *observer) init() {
	objList, rev := o.readRemote()
	o.rev = rev
	for _, obj := range objList {
		if obj.Key == "" {
			continue
		}
		key := obj.Key
		v, _, err := o.newViper(key, obj.Content)
		if err != nil {
			continue
		}
		o.cache[key] = v
	}
	go o.watchRemote()
}

type contentVal struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

func getViper(key string) (*viper.Viper, bool) {
	v, b := ob.getViper(key)
	if !b {
		logger.Logger.Errorf("no dynamic viper: %s", key)
	}
	return v, b
}

func GetString(key, path string) string {
	v, b := getViper(key)
	if !b {
		return ""
	}
	return v.GetString(path)
}

func GetInt(key, path string) int {
	v, b := getViper(key)
	if !b {
		return 0
	}
	return v.GetInt(path)
}

func Get(key, path string) any {
	v, b := getViper(key)
	if !b {
		return nil
	}
	return v.Get(path)
}

func GetBool(key, path string) bool {
	v, b := getViper(key)
	if !b {
		return false
	}
	return v.GetBool(path)
}

func GetFloat64(key, path string) float64 {
	v, b := getViper(key)
	if !b {
		return 0
	}
	return v.GetFloat64(path)
}

func GetStringMapString(key, path string) map[string]string {
	v, b := getViper(key)
	if !b {
		return make(map[string]string)
	}
	return v.GetStringMapString(path)
}

func GetStringMap(key, path string) map[string]any {
	v, b := getViper(key)
	if !b {
		return make(map[string]any)
	}
	return v.GetStringMap(path)
}

func Exists(key, path string) bool {
	v, b := getViper(key)
	if !b {
		return false
	}
	return v.IsSet(path)
}

func GetInt64(key, path string) int64 {
	v, b := getViper(key)
	if !b {
		return 0
	}
	return v.GetInt64(path)
}

func GetMapSlice(key, path string) []map[string]any {
	ret := Get(key, path)
	if ret == nil {
		return []map[string]any{}
	}
	r := reflect.ValueOf(ret)
	switch r.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return []map[string]any{}
	}
	obj := make([]map[string]any, 0, r.Len())
	for i := 0; i < r.Len(); i++ {
		item := r.Index(i).Interface()
		ir := reflect.ValueOf(item)
		if ir.Kind() == reflect.Map && ir.Type().Key().Kind() == reflect.String {
			m := make(map[string]any)
			keys := ir.MapKeys()
			for _, k := range keys {
				m[k.String()] = ir.MapIndex(k).Interface()
			}
			obj = append(obj, m)
		}
	}
	return obj
}
