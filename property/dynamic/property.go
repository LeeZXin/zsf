package dynamic

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/quit"
	_ "github.com/LeeZXin/zsf-utils/sentinelutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/ext/datasource"
	"github.com/spf13/viper"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"
)

const (
	flowJsonPath           = "sentinel-flow.json"
	circuitBreakerJsonPath = "sentinel-circuitbreaker.json"
)

var (
	loader *propertyLoader
)

// Init 初始化
func Init() {
	if loader == nil {
		loader = newPropertyLoader()
	}
}

type container struct {
	*viper.Viper
	Raw Content
}

type propertyLoader struct {
	sync.RWMutex
	cache      map[string]*container
	client     *clientv3.Client
	key        string
	ctx        context.Context
	cancelFunc context.CancelFunc
	rev        int64

	sentinelFlowBase           datasource.PropertyHandler
	sentinelCircuitBreakerBase datasource.PropertyHandler
}

func (o *propertyLoader) Close() {
	logger.Logger.Infof("dynamic property observer closed")
	o.cancelFunc()
	o.client.Close()
	o.Lock()
	defer o.Unlock()
	o.cache = nil
}

func newPropertyLoader() *propertyLoader {
	o := new(propertyLoader)
	// for sentinel
	o.sentinelFlowBase = datasource.NewFlowRulesHandler(datasource.FlowRuleJsonArrayParser)
	o.sentinelCircuitBreakerBase = datasource.NewCircuitBreakerRulesHandler(datasource.CircuitBreakerRuleJsonArrayParser)
	o.cache = make(map[string]*container, 8)
	o.key = common.PropertyPrefix + common.GetApplicationName() + "/"
	var err error
	o.client, err = clientv3.New(clientv3.Config{
		Endpoints:        strings.Split(static.GetString("property.dynamic.etcd.hosts"), ";"),
		AutoSyncInterval: time.Minute,
		DialTimeout:      10 * time.Second,
		Username:         static.GetString("property.dynamic.etcd.username"),
		Password:         static.GetString("property.dynamic.etcd.dyna"),
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

func (o *propertyLoader) readRemote() ([]*mvccpb.KeyValue, int64) {
	response, err := o.client.Get(o.ctx, o.key, clientv3.WithPrefix())
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			logger.Logger.Fatalf("etcd dynamic property permission denied")
		}
		logger.Logger.Error(err)
		return nil, 0
	}
	return response.Kvs, response.Header.GetRevision()
}

func (o *propertyLoader) watchRemote() {
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

func (o *propertyLoader) deleteKey(key string) {
	o.Lock()
	defer o.Unlock()
	delete(o.cache, key)
}

func (o *propertyLoader) putKey(key string, v *container) {
	o.Lock()
	defer o.Unlock()
	o.cache[key] = v
}

func (o *propertyLoader) getContainer(key string) (*container, bool) {
	o.RLock()
	defer o.RUnlock()
	ret, b := o.cache[key]
	return ret, b
}

func (o *propertyLoader) dealChan(wchan clientv3.WatchChan) {
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
					o.convertAndHandleDelete(event.Kv)
				case clientv3.EventTypePut:
					o.convertAndHandlePut(event.Kv)
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

func (o *propertyLoader) loadOrNewContainer(key string, val Content) (*container, bool) {
	v, b := o.getContainer(key)
	if !b {
		v = &container{
			Viper: viper.New(),
			Raw:   val,
		}
		v.SetConfigType(ext(key))
	} else {
		v.Raw = val
	}
	// 忽略转化异常
	v.MergeConfig(strings.NewReader(val.Content))
	logger.Logger.Infof("merge remote config successfully key: %s, version: %s", key, val.Version)
	return v, b
}

func (o *propertyLoader) init() {
	kvs, rev := o.readRemote()
	o.rev = rev
	for _, kv := range kvs {
		o.convertAndHandlePut(kv)
	}
	go o.watchRemote()
}

func (o *propertyLoader) convertAndHandlePut(kv *mvccpb.KeyValue) {
	if kv == nil {
		return
	}
	key := strings.TrimPrefix(string(kv.Key), o.key)
	var val Content
	err := json.Unmarshal(kv.Value, &val)
	if err != nil {
		logger.Logger.Errorf("read remote config is not json format: %s", key)
	} else if val.Version == "" {
		logger.Logger.Errorf("read remote config version is empty: %s %v", key, val)
	} else {
		o.handlePut(key, val)
		// 通知监听
		notifyListener(key, val, PutEventType)
	}
}

func (o *propertyLoader) convertAndHandleDelete(kv *mvccpb.KeyValue) {
	if kv == nil {
		return
	}
	key := strings.TrimPrefix(string(kv.Key), o.key)
	o.handleDelete(key)
	// 通知监听
	notifyListener(key, Content{}, DeleteEventType)
}

func (o *propertyLoader) handlePut(key string, val Content) {
	switch key {
	case flowJsonPath:
		o.sentinelFlowBase.Handle([]byte(val.Content))
	case circuitBreakerJsonPath:
		o.sentinelCircuitBreakerBase.Handle([]byte(val.Content))
	default:
		v, b := o.loadOrNewContainer(key, val)
		if !b {
			o.putKey(key, v)
		}
	}
}

func (o *propertyLoader) handleDelete(key string) {
	switch key {
	case flowJsonPath:
		flow.LoadRules(nil)
	case circuitBreakerJsonPath:
		circuitbreaker.LoadRules(nil)
	default:
		logger.Logger.Infof("delete dynamic key: %s", key)
		o.deleteKey(key)
	}
}

type Content struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

func getContainer(key string) (*container, bool) {
	if loader == nil {
		return nil, false
	}
	v, b := loader.getContainer(key)
	if !b {
		logger.Logger.Errorf("no dynamic viper: %s", key)
	}
	return v, b
}

func GetIntSlice(key, path string) []int {
	v, b := getContainer(key)
	if !b {
		return nil
	}
	return v.GetIntSlice(path)
}

func GetStringSlice(key, path string) []string {
	v, b := getContainer(key)
	if !b {
		return nil
	}
	return v.GetStringSlice(path)
}

func GetString(key, path string) string {
	v, b := getContainer(key)
	if !b {
		return ""
	}
	return v.GetString(path)
}

func GetInt(key, path string) int {
	v, b := getContainer(key)
	if !b {
		return 0
	}
	return v.GetInt(path)
}

func GetRawContent(key string) (Content, bool) {
	v, b := getContainer(key)
	if !b {
		return Content{}, false
	}
	return v.Raw, true
}

func Get(key, path string) any {
	v, b := getContainer(key)
	if !b {
		return nil
	}
	return v.Get(path)
}

func GetBool(key, path string) bool {
	v, b := getContainer(key)
	if !b {
		return false
	}
	return v.GetBool(path)
}

func GetFloat64(key, path string) float64 {
	v, b := getContainer(key)
	if !b {
		return 0
	}
	return v.GetFloat64(path)
}

func GetStringMapString(key, path string) map[string]string {
	v, b := getContainer(key)
	if !b {
		return make(map[string]string)
	}
	return v.GetStringMapString(path)
}

func GetStringMap(key, path string) map[string]any {
	v, b := getContainer(key)
	if !b {
		return make(map[string]any)
	}
	return v.GetStringMap(path)
}

func Exists(key, path string) bool {
	v, b := getContainer(key)
	if !b {
		return false
	}
	return v.IsSet(path)
}

func GetInt64(key, path string) int64 {
	v, b := getContainer(key)
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
