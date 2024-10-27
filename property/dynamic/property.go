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
	defaultLoader *Loader
)

// InitDefault 初始化
func InitDefault() {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:        strings.Split(static.GetString("property.dynamic.etcd.endpoints"), ";"),
		AutoSyncInterval: time.Minute,
		DialTimeout:      10 * time.Second,
		Username:         static.GetString("property.dynamic.etcd.username"),
		Password:         static.GetString("property.dynamic.etcd.password"),
		Logger:           zap.NewNop(),
	})
	if err != nil {
		logger.Logger.Fatalf("init dynamic.etcd.client failed with err: %v", err)
	}
	defaultLoader = NewLoader("", client)
}

type container struct {
	*viper.Viper
	Raw Content
}

type Loader struct {
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

func (l *Loader) Close() {
	logger.Logger.Infof("dynamic property observer closed")
	l.cancelFunc()
	l.client.Close()
	l.Lock()
	defer l.Unlock()
	l.cache = nil
}

func NewLoader(applicationName string, etcdClient *clientv3.Client) *Loader {
	if etcdClient == nil {
		logger.Logger.Fatal("new dynamic.loader with nil etcd client")
	}
	if applicationName == "" {
		applicationName = common.GetApplicationName()
	}
	loader := new(Loader)
	// for sentinel
	loader.sentinelFlowBase = datasource.NewFlowRulesHandler(datasource.FlowRuleJsonArrayParser)
	loader.sentinelCircuitBreakerBase = datasource.NewCircuitBreakerRulesHandler(datasource.CircuitBreakerRuleJsonArrayParser)
	loader.cache = make(map[string]*container, 8)
	loader.key = common.PropertyPrefix + applicationName + "/"
	loader.client = etcdClient
	loader.ctx, loader.cancelFunc = context.WithCancel(context.Background())
	quit.AddShutdownHook(loader.Close)
	logger.Logger.Infof("start listening dynamic property key: %s", loader.key)
	loader.init()
	return loader
}

func (l *Loader) readRemote() ([]*mvccpb.KeyValue, int64) {
	response, err := l.client.Get(l.ctx, l.key, clientv3.WithPrefix())
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			logger.Logger.Fatalf("etcd dynamic property permission denied")
		}
		logger.Logger.Error(err)
		return nil, 0
	}
	return response.Kvs, response.Header.GetRevision()
}

func (l *Loader) watchRemote() {
	for {
		if l.ctx.Err() != nil {
			return
		}
		logger.Logger.Infof("try to watch prefix: %s with revision: %d", l.key, l.rev+1)
		watcher := clientv3.NewWatcher(l.client)
		wchan := watcher.Watch(l.ctx, l.key, clientv3.WithPrefix(), clientv3.WithRev(l.rev+1))
		l.dealChan(wchan)
		watcher.Close()
		time.Sleep(10 * time.Second)
	}
}

func (l *Loader) deleteKey(key string) {
	l.Lock()
	defer l.Unlock()
	delete(l.cache, key)
}

func (l *Loader) putKey(key string, v *container) {
	l.Lock()
	defer l.Unlock()
	l.cache[key] = v
}

func (l *Loader) getContainer(key string) (*container, bool) {
	l.RLock()
	defer l.RUnlock()
	ret, b := l.cache[key]
	return ret, b
}

func (l *Loader) dealChan(wchan clientv3.WatchChan) {
	for {
		select {
		case <-l.ctx.Done():
			return
		case data, ok := <-wchan:
			if !ok || data.Canceled {
				logger.Logger.Info("dynamic property is canceled")
				if err := data.Err(); err != nil {
					logger.Logger.Error(err)
				}
				return
			}
			l.rev = data.Header.Revision
			for _, event := range data.Events {
				switch event.Type {
				case clientv3.EventTypeDelete:
					l.convertAndHandleDelete(event.Kv)
				case clientv3.EventTypePut:
					l.convertAndHandlePut(event.Kv)
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

func (l *Loader) loadOrNewContainer(key string, val Content) (*container, bool) {
	v, b := l.getContainer(key)
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
	return v, b
}

func (l *Loader) init() {
	kvs, rev := l.readRemote()
	l.rev = rev
	for _, kv := range kvs {
		l.convertAndHandlePut(kv)
	}
	go l.watchRemote()
}

func (l *Loader) convertAndHandlePut(kv *mvccpb.KeyValue) {
	if kv == nil {
		return
	}
	key := strings.TrimPrefix(string(kv.Key), l.key)
	var val Content
	err := json.Unmarshal(kv.Value, &val)
	if err != nil {
		logger.Logger.Errorf("read remote config is not json format: %s", key)
	} else if val.Version == "" {
		logger.Logger.Errorf("read remote config version is empty: %s %v", key, val)
	} else {
		l.handlePut(key, val)
		// 通知监听
		notifyListener(key, val, PutEventType)
	}
}

func (l *Loader) convertAndHandleDelete(kv *mvccpb.KeyValue) {
	if kv == nil {
		return
	}
	key := strings.TrimPrefix(string(kv.Key), l.key)
	l.handleDelete(key)
	// 通知监听
	notifyListener(key, Content{}, DeleteEventType)
}

func (l *Loader) handlePut(key string, val Content) {
	logger.Logger.Infof("merge remote config successfully key: %s, version: %s", key, val.Version)
	switch key {
	case flowJsonPath:
		err := l.sentinelFlowBase.Handle([]byte(val.Content))
		if err != nil {
			logger.Logger.Errorf("handle put %s failed with err: %v", flowJsonPath, err)
		}
	case circuitBreakerJsonPath:
		err := l.sentinelCircuitBreakerBase.Handle([]byte(val.Content))
		if err != nil {
			logger.Logger.Errorf("handle put %s failed with err: %v", circuitBreakerJsonPath, err)
		}
	default:
		v, b := l.loadOrNewContainer(key, val)
		if !b {
			l.putKey(key, v)
		}
	}
}

func (l *Loader) handleDelete(key string) {
	switch key {
	case flowJsonPath:
		flow.LoadRules(nil)
	case circuitBreakerJsonPath:
		circuitbreaker.LoadRules(nil)
	default:
		logger.Logger.Infof("delete dynamic key: %s", key)
		l.deleteKey(key)
	}
}

type Content struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

func (l *Loader) GetIntSlice(key, path string) []int {
	v, b := l.getContainer(key)
	if !b {
		return nil
	}
	return v.GetIntSlice(path)
}

func (l *Loader) GetStringSlice(key, path string) []string {
	v, b := l.getContainer(key)
	if !b {
		return nil
	}
	return v.GetStringSlice(path)
}

func (l *Loader) GetString(key, path string) string {
	v, b := l.getContainer(key)
	if !b {
		return ""
	}
	return v.GetString(path)
}

func (l *Loader) GetInt(key, path string) int {
	v, b := l.getContainer(key)
	if !b {
		return 0
	}
	return v.GetInt(path)
}

func (l *Loader) GetRawContent(key string) (Content, bool) {
	v, b := l.getContainer(key)
	if !b {
		return Content{}, false
	}
	return v.Raw, true
}

func (l *Loader) Get(key, path string) any {
	v, b := l.getContainer(key)
	if !b {
		return nil
	}
	return v.Get(path)
}

func (l *Loader) GetBool(key, path string) bool {
	v, b := l.getContainer(key)
	if !b {
		return false
	}
	return v.GetBool(path)
}

func (l *Loader) GetFloat64(key, path string) float64 {
	v, b := l.getContainer(key)
	if !b {
		return 0
	}
	return v.GetFloat64(path)
}

func (l *Loader) GetStringMapString(key, path string) map[string]string {
	v, b := l.getContainer(key)
	if !b {
		return make(map[string]string)
	}
	return v.GetStringMapString(path)
}

func (l *Loader) GetStringMap(key, path string) map[string]any {
	v, b := l.getContainer(key)
	if !b {
		return make(map[string]any)
	}
	return v.GetStringMap(path)
}

func (l *Loader) Exists(key, path string) bool {
	v, b := l.getContainer(key)
	if !b {
		return false
	}
	return v.IsSet(path)
}

func (l *Loader) GetInt64(key, path string) int64 {
	v, b := l.getContainer(key)
	if !b {
		return 0
	}
	return v.GetInt64(path)
}

func (l *Loader) GetMapSlice(key, path string) []map[string]any {
	ret := l.Get(key, path)
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

func GetIntSlice(key, path string) []int {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.GetIntSlice(key, path)
}

func GetStringSlice(key, path string) []string {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.GetStringSlice(key, path)
}

func GetString(key, path string) string {
	if defaultLoader == nil {
		return ""
	}
	return defaultLoader.GetString(key, path)
}

func GetInt(key, path string) int {
	if defaultLoader == nil {
		return 0
	}
	return defaultLoader.GetInt(key, path)
}

func GetRawContent(key string) (Content, bool) {
	if defaultLoader == nil {
		return Content{}, false
	}
	return defaultLoader.GetRawContent(key)
}

func Get(key, path string) any {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.Get(key, path)
}

func GetBool(key, path string) bool {
	if defaultLoader == nil {
		return false
	}
	return defaultLoader.GetBool(key, path)
}

func GetFloat64(key, path string) float64 {
	if defaultLoader == nil {
		return 0
	}
	return defaultLoader.GetFloat64(key, path)
}

func GetStringMapString(key, path string) map[string]string {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.GetStringMapString(key, path)
}

func GetStringMap(key, path string) map[string]any {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.GetStringMap(key, path)
}

func Exists(key, path string) bool {
	if defaultLoader == nil {
		return false
	}
	return defaultLoader.Exists(key, path)
}

func GetInt64(key, path string) int64 {
	if defaultLoader == nil {
		return 0
	}
	return defaultLoader.GetInt64(key, path)
}

func GetMapSlice(key, path string) []map[string]any {
	if defaultLoader == nil {
		return nil
	}
	return defaultLoader.GetMapSlice(key, path)
}
