package static

import (
	"fmt"
	"github.com/LeeZXin/zsf/env"
	"github.com/spf13/viper"
	"reflect"
)

// 获取配置信息
// 固定程序下 resources/application.yaml路径
// 封装viper
// 实现多环境配置

var (
	v *viper.Viper
)

// init 配置优先顺序 application-{dev}.yaml > node-{nodeFlag}.yaml > application.yaml
func init() {
	// 最终配置
	v = viper.New()
	//默认加载/resources/application.yaml
	v1 := viper.New()
	v1.SetConfigType("yaml")
	v1.AddConfigPath("resources")
	v1.SetConfigName("application.yaml")
	_ = v1.ReadInConfig()
	for _, k := range v1.AllKeys() {
		v.SetDefault(k, v1.Get(k))
	}
	// 加载集群标记不同的配置信息
	if env.GetNodeFlag() != "" {
		v2 := viper.New()
		v2.SetConfigType("yaml")
		v2.AddConfigPath("resources")
		// 注意是node-开头 与application-区分开
		v2.SetConfigName(fmt.Sprintf("node-%s.yaml", env.GetNodeFlag()))
		_ = v2.ReadInConfig()
		for _, k := range v2.AllKeys() {
			v.SetDefault(k, v2.Get(k))
		}
	}
	//根据环境配置加载/resources/application-{env}.yaml
	//覆盖上面默认配置
	v.SetConfigType("yaml")
	v.AddConfigPath("resources")
	v.SetConfigName(fmt.Sprintf("application-%s.yaml", env.GetEnv()))
	_ = v.ReadInConfig()
}

func GetIntSlice(key string) []int {
	return v.GetIntSlice(key)
}

func GetStringSlice(key string) []string {
	return v.GetStringSlice(key)
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

func Exists(key string) bool {
	return v.IsSet(key)
}

func GetInt64(key string) int64 {
	return v.GetInt64(key)
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
