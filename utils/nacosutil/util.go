// Package nacosutil 提供 Nacos 客户端参数构造工具，
// 配置项（server-addr、namespace、分组）均来自静态配置（config/static）。
package nacosutil

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/start"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacoslogger "github.com/nacos-group/nacos-sdk-go/v2/common/logger"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func init() {
	start.AddInit(func() {
		nacoslogger.SetLogger(new(nlogger))
	}, -6)
}

// MustNewClientParam 从静态配置读取 nacos.server-addr（host:port）构造 Nacos 客户端参数，
// namespace 为 "public" 时归一化为空字符串（Nacos 默认命名空间的内部表示）。
// 注意：配置缺失或格式非法时进程 Fatal 退出，调用方无法捕获该错误。
func MustNewClientParam() vo.NacosClientParam {
	serverAddr := static.GetString("nacos.server-addr")
	if serverAddr == "" {
		logger.Logger.Fatal().Msg("nacos.server-addr is empty")
	}
	namespace := static.GetString("nacos.namespace")
	// Nacos 的默认命名空间对外叫 public、对内是空字符串，统一归一化。
	if namespace == "public" {
		namespace = ""
	}
	host, portStr, err := net.SplitHostPort(serverAddr)
	if err != nil {
		logger.Logger.Fatal().Msgf("nacos.server-addr is invalid: %v", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		logger.Logger.Fatal().Msgf("nacos.server-addr is invalid: %v", err)
	}
	serverConfigs := []constant.ServerConfig{
		*constant.NewServerConfig(host, port, constant.WithContextPath("/nacos")),
	}
	clientConfig := constant.NewClientConfig(
		constant.WithNamespaceId(namespace),
		constant.WithTimeoutMs(5000),
		constant.WithAppName(instance.ApplicationName),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithUpdateCacheWhenEmpty(true),
		constant.WithCacheDir("/tmp/nacos-sdk-go/"+instance.ApplicationName+"/cache"),
		constant.WithAccessKey(static.GetString("nacos.access-key")),
		constant.WithSecretKey(static.GetString("nacos.secret-key")),
		// 默认鉴权插件用 username/password；未配则 SDK 不登录（对应服务端关闭鉴权）。
		constant.WithUsername(static.GetString("nacos.username")),
		constant.WithPassword(static.GetString("nacos.password")),
	)
	return vo.NacosClientParam{
		ClientConfig:  clientConfig,
		ServerConfigs: serverConfigs,
	}
}

// ClientParamForNamespace 基于 MustNewClientParam 拷贝一份参数并切换命名空间。
// public / 空串归一成 SDK 内部空字符串；缓存与日志目录按命名空间隔离，避免多 Client 抢同一份本地缓存。
func ClientParamForNamespace(namespaceId string) vo.NacosClientParam {
	param := MustNewClientParam()
	cc := *param.ClientConfig
	nsKey := strings.TrimSpace(namespaceId)
	if nsKey == "" || nsKey == "public" {
		cc.NamespaceId = ""
		nsKey = "public"
	} else {
		cc.NamespaceId = nsKey
	}
	cc.CacheDir = filepath.Join(cc.CacheDir, nsKey)
	cc.LogDir = filepath.Join(cc.LogDir, nsKey)
	param.ClientConfig = &cc
	return param
}

// GetGroup 从静态配置读取分组名，未配置或为空时返回 DefaultGroup。
func GetGroup(key string) string {
	group := static.GetString(key)
	if group == "" {
		group = constant.DEFAULT_GROUP
	}
	return group
}
