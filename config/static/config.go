// Package static 提供基于 viper 的静态配置加载与读取，封装 resources/ 目录下的 yaml 文件。
//
// 加载规则（由 start 钩子 -9 在启动时一次性完成，进程运行期不可变）：
//   - 基础配置 resources/application.yaml，所有环境共用
//   - 集群配置 resources/cluster-{cluster}.yaml，按 env.GetCluster() 加载，覆盖基础配置
//   - 环境配置 resources/application-{env}.yaml，按 env.Env 加载，优先级最高
//
// 优先级：application-{env}.yaml > cluster-{cluster}.yaml > application.yaml，
// 即上层文件的同名 key 覆盖下层。文件不存在或 key 缺失时静默忽略，Get 系函数返回零值。
//
// 职责边界：只负责静态配置（启动后不变）；运行期热更新的配置见 config/dynamic。
package static

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/start"

	"github.com/spf13/viper"
)

var (
	v   *viper.Viper
	dir string
)

// init 注册配置加载钩子（order -9）。
// 优先级从低到高：application.yaml → cluster-{cluster}.yaml → application-{env}.yaml，
// 后加载者覆盖先加载者的同名 key（SetDefault 按序覆盖 + ReadInConfig 覆盖 defaults）。
func init() {
	start.AddInit(Init, -9)
}

// Init 加载静态 yaml。已加载则直接返回。
// resources/ 从 cwd 向上找，再试可执行文件目录（git-hook 的 cwd 是裸仓）。
func Init() {
	if v != nil {
		return
	}
	v = viper.New()
	if base := findResourcesBase(); base != "" {
		dir = base
	} else if wd, err := os.Getwd(); err == nil {
		dir = wd
	}
	res := "resources"
	if dir != "" {
		res = filepath.Join(dir, "resources")
	}
	v1 := viper.New()
	v1.SetConfigType("yaml")
	v1.AddConfigPath(res)
	v1.SetConfigName("application.yaml")
	_ = v1.ReadInConfig()
	for _, k := range v1.AllKeys() {
		v.SetDefault(k, v1.Get(k))
	}
	if env.GetCluster() != "" {
		v2 := viper.New()
		v2.SetConfigType("yaml")
		v2.AddConfigPath(res)
		v2.SetConfigName(fmt.Sprintf("cluster-%s.yaml", env.GetCluster()))
		_ = v2.ReadInConfig()
		for _, k := range v2.AllKeys() {
			v.SetDefault(k, v2.Get(k))
		}
	}
	if env.Env != "" {
		v.SetConfigType("yaml")
		v.AddConfigPath(res)
		v.SetConfigName(fmt.Sprintf("application-%s.yaml", env.Env))
		_ = v.ReadInConfig()
	}
}

// Dir 返回找到 resources/application.yaml 的目录。
// git-hook 不 chdir，相对路径（git.repo-root、sqlite 文件）应基于此解析。
func Dir() string {
	return dir
}

func findResourcesBase() string {
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
		p := wd
		for range 10 {
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			candidates = append(candidates, parent)
			p = parent
		}
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		candidates = append(candidates, filepath.Dir(exe))
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "resources", "application.yaml")); err != nil {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			return c
		}
		return abs
	}
	return ""
}

// GetIntSlice 读取 key 对应的 []int 配置。
// key 使用点分隔路径（如 "http.ports"）；缺失或类型不符时返回 nil/零值。
func GetIntSlice(key string) []int {
	return v.GetIntSlice(key)
}

// GetStringSlice 读取 key 对应的 []string 配置。
// key 使用点分隔路径；缺失或类型不符时返回 nil。
func GetStringSlice(key string) []string {
	return v.GetStringSlice(key)
}

// GetString 读取 key 对应的字符串配置。
// key 使用点分隔路径；缺失时返回空串。
func GetString(key string) string {
	return v.GetString(key)
}

// GetInt 读取 key 对应的 int 配置。
// key 使用点分隔路径；缺失或类型不符时返回 0。
// 注意：值为数字字符串时 viper 会尝试类型转换，类型无法转换时返回 0 且不报错。
func GetInt(key string) int {
	return v.GetInt(key)
}

// Get 读取 key 对应的原始值（any），保留 yaml 中的原始类型。
// key 使用点分隔路径；缺失时返回 nil。
func Get(key string) any {
	return v.Get(key)
}

// GetBool 读取 key 对应的 bool 配置。
// key 使用点分隔路径；缺失或类型不符时返回 false。
func GetBool(key string) bool {
	return v.GetBool(key)
}

// GetFloat64 读取 key 对应的 float64 配置。
// key 使用点分隔路径；缺失或类型不符时返回 0。
func GetFloat64(key string) float64 {
	return v.GetFloat64(key)
}

// GetStringMapString 读取 key 对应的 map[string]string 配置。
// key 使用点分隔路径；缺失或类型不符时返回 nil。
func GetStringMapString(key string) map[string]string {
	return v.GetStringMapString(key)
}

// GetStringMap 读取 key 对应的 map[string]any 配置。
// key 使用点分隔路径；缺失或类型不符时返回 nil。
func GetStringMap(key string) map[string]any {
	return v.GetStringMap(key)
}

// Exists 判断 key 是否在配置中显式存在（包括来自各层 yaml 与默认值）。
func Exists(key string) bool {
	return v.IsSet(key)
}

// GetInt64 读取 key 对应的 int64 配置。
// key 使用点分隔路径；缺失或类型不符时返回 0。
func GetInt64(key string) int64 {
	return v.GetInt64(key)
}

// GetDuration 读取 key 对应的 duration 配置。
// key 使用点分隔路径；支持 yaml 中的字符串形式（如 "5s"、"1m"）；
// 缺失或解析失败时返回 0。
func GetDuration(key string) time.Duration {
	return v.GetDuration(key)
}

// GetMapSlice 读取 key 对应的 []map[string]any 配置（如 yaml 中的对象数组）。
// key 使用点分隔路径；缺失或元素不是字符串键 map 的元素会被跳过，
// 全部不合法时返回空切片（而非 nil）。
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
