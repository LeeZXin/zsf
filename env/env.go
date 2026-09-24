/*
Package env 提供全局环境变量注册与读取。

通过 start.AddInit 注册启动钩子（order -10，zsf 框架最先执行），
读取 SF_ENV 和 SF_CLUSTER 环境变量，并获取本机第一个非回环 IPv4 地址存入 LocalIP。
这些值在进程启动后不可变，为整个框架提供统一的环境判断能力。

环境划分：
  - sit: 测试环境（默认值）
  - prd: 生产环境
  - dev / staging 等按需自定义

使用示例:

	if env.IsSitEnv() {
	    // 测试环境特殊逻辑
	}
	logger.Logger.Info().Str("ip", env.LocalIP).Msg("service started")
*/
package env

import (
	"os"

	"github.com/LeeZXin/zsf/utils/iputil"
)

/*
全局环境变量存储。

Env: 当前运行环境标识，通过 SF_ENV 环境变量配置，默认值为 "sit"。
Cluster: 集群标识，用于多集群部署场景（如华北/华南），通过 SF_CLUSTER 环境变量配置。空值表示不区分集群。
LocalIP: 本机第一个非回环 IPv4 地址，用于日志标识和 Prometheus push 的 instance 标签。
*/
var (
	Env     string
	Cluster string
	LocalIP string
)

const (
	/*
		Sit 测试环境标识
	*/
	SitEnv = "sit"
	/*
		Prd 生产环境标识
	*/
	PrdEnv = "prd"
	/*
	   Debug 标识
	*/
	DebugEnv = "debug"
)

// init 读取 SF_ENV / SF_CLUSTER 与本机 IP。已执行过则直接返回。
func init() {
	if Env != "" {
		return
	}
	Env = os.Getenv("SF_ENV")
	Cluster = os.Getenv("SF_CLUSTER")
	LocalIP = iputil.GetIPV4()
}

/*
GetCluster 返回当前集群标识。
未配置 SF_CLUSTER 时返回空字符串，表示单集群部署。
*/
func GetCluster() string {
	return Cluster
}

/*
IsSitEnv 判断当前是否为 SIT（系统集成测试）环境。
*/
func IsSitEnv() bool {
	return Env == SitEnv
}

/*
IsDebugEnv 判断当前是否为 DEBUG 环境。
*/
func IsDebugEnv() bool {
	return Env == DebugEnv
}

/*
IsPrdEnv 判断当前是否为生产环境。
*/
func IsPrdEnv() bool {
	return Env == PrdEnv
}
