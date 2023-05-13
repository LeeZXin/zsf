package property

import (
	"fmt"
	"github.com/spf13/viper"
	"io"
	"zsf/common"
)

// 获取配置信息
// 固定程序下 resources/application.yaml路径
// 封装viper
// 实现多环境配置

var (
	v *viper.Viper
)

func init() {
	v1 := viper.New()
	v1.SetConfigType("yaml")
	v1.AddConfigPath("./resources/")
	v1.SetConfigName("application.yaml")
	_ = v1.ReadInConfig()
	v = viper.New()
	for k, s := range v1.AllSettings() {
		v.SetDefault(k, s)
	}
	v.SetConfigType("yaml")
	v.AddConfigPath("./resources/")
	cn := fmt.Sprintf("application-%s.yaml", common.Env)
	v.SetConfigName(cn)
	_ = v.ReadInConfig()
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
