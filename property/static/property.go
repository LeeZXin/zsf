package static

import (
	"fmt"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/spf13/viper"
	"io"
)

// 获取配置信息
// 固定程序下 resources/application.yaml路径
// 封装viper
// 实现多环境配置

var (
	v *viper.Viper
)

func init() {
	//默认加载/resources/application.yaml
	v1 := viper.New()
	v1.SetConfigType("yaml")
	v1.AddConfigPath("./resources/")
	v1.SetConfigName("application.yaml")
	_ = v1.ReadInConfig()
	v = viper.New()
	for k, s := range v1.AllSettings() {
		v.SetDefault(k, s)
	}
	//根据环境配置加载/resources/application-{env}.yaml
	//覆盖上面默认配置
	v.SetConfigType("yaml")
	v.AddConfigPath("./resources/")
	v.SetConfigName(fmt.Sprintf("application-%s.yaml", cmd.GetEnv()))
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

func GetInt64(key string) int64 {
	return v.GetInt64(key)
}
