package dynamic

import (
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/atomicutil"
	"github.com/LeeZXin/zsf-utils/maputil"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/zsf"
	"github.com/spf13/viper"
	"io"
	"reflect"
)

var (
	v              *viper.Viper
	chosenProperty = atomicutil.NewValue[Property]()
	propertyMap    = maputil.NewImmutableMap(map[string]Property{
		defaultImpl.GetPropertyType(): defaultImpl,
		consulImpl.GetPropertyType():  consulImpl,
	})
)

type Property interface {
	GetPropertyType() string
	OnKeyChange(string, KeyChangeCallback)
	zsf.LifeCycle
}

func init() {
	v = viper.New()
	v.SetConfigType("yaml")
	SetProperty(propertyMap.GetOrDefault(static.GetString("property.dynamic.type"), defaultImpl))
}

type KeyChangeCallback func()

// getAllProperties 获取配置
func getAllProperties(listenKeys []string) map[string]string {
	properties := make(map[string]string, len(listenKeys))
	for _, key := range listenKeys {
		oldProperty := Get(key)
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
func OnKeyChange(key string, callback KeyChangeCallback) {
	if key == "" || callback == nil {
		return
	}
	p := propertyMap.GetOrDefault(static.GetString("property.dynamic.type"), defaultImpl)
	p.OnKeyChange(key, callback)
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

func AllSettings() map[string]any {
	return v.AllSettings()
}

func GetMapSlice(key string) []map[string]any {
	ret := Get(key)
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

func SetProperty(property Property) {
	if property == nil {
		return
	}
	zsf.RegisterApplicationLifeCycle(property)
	has, b := chosenProperty.Load()
	if b {
		has.OnApplicationShutdown()
	}
	chosenProperty.Store(property)
}
