// Package dynamic 提供基于 Nacos 的动态配置注册与读取。
//
// 规范：
//   - 全平台所有服务共用同一个 namespace（nacos.namespace 配置），禁止各自为政
//   - 配置分组统一为 nacos.group（默认 DEFAULT_GROUP）
//   - 服务通过 Register(dataId) 获取一个 *Value[T]，随时 .Get() 读取最新值；
//     Nacos 侧变更会热更新（SDK 长轮询推送）
//   - fail-fast：Register 会同步拉取配置，拉取失败直接 Fatal——
//     配置必须提前在控制台创建，且服务启动时 Nacos 必须可用
//
// 配置键（static 包，resources/application-*.yaml）：
//   - nacos.namespace:   命名空间 ID；缺省或 "public" 均表示默认命名空间
//   - nacos.server-addr: Nacos 服务地址（host:port），必配
//   - nacos.group:       配置分组，默认 DEFAULT_GROUP
package dynamic

import (
	"time"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/utils/nacosutil"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Decoder 把配置内容（字符串）解码为业务类型 V。
type Decoder[V any] func(string) V

// DefaultDecoder 是 Decoder[string] 的默认实现（原样返回字符串）。
// 配置内容即字符串本身时可直接传入，无需自定义解码。
var DefaultDecoder Decoder[string] = func(s string) string {
	return s
}

// Callback[V any] 是配置变更回调函数类型。
// 通过 Register 注册，在配置首次监听送达及每次 Nacos 热更新时触发，
// 参数为解码后的最新值。回调在 Nacos SDK 的监听 goroutine 内同步执行，
// 实现方不应做耗时操作，也不应阻塞（会影响同 client 的其他监听）。
type Callback[V any] func(V)

// configClient 是全局唯一的 Nacos config client（start 钩子中创建，创建失败直接 Fatal）。
var configClient config_client.IConfigClient

var group string

/*
init 注册 Nacos config client 初始化钩子（order -3，zsf 框架最后，
依赖 static -9 加载 yaml、nacosutil -6 安装 SDK 日志适配器）；
初始化失败直接 Fatal：配置中心是基础设施，带病启动没有意义。
*/
func init() {
	start.AddInit(func() {
		group = nacosutil.GetGroup("nacos.config.group")
		clientParam := nacosutil.MustNewClientParam()
		client, err := clients.NewConfigClient(clientParam)
		if err != nil {
			logger.Logger.Fatal().Msgf("failed to init nacos config client: %v", err)
		}
		quit.AddFinalShutdownHook(client.CloseClient)
		configClient = client
		logger.Logger.Info().Msgf("dynamic: nacos config client ready, namespace=%s server=%s:%d",
			clientParam.ClientConfig.NamespaceId, clientParam.ServerConfigs[0].IpAddr, clientParam.ServerConfigs[0].Port)
		// 等待 gRPC 连接就绪：SDK 异步建连（状态 STARTING 窗口实测常见 <300ms、极端 ~2s），
		// 未就绪时 GetConfig 会快速失败（SDK 内置重试约 300ms 内放弃）。
		// 睡眠 1s + SDK 内置重试可覆盖绝大多数启动场景；极端慢启动直接让后续
		// Register 的 GetConfig 触发 Fatal（fail-fast：此时 Nacos 或本机状态确实有问题）。
		// 代价：每个服务启动固定 +1s。
		time.Sleep(time.Second)
	}, -3)
}

/*
Register 注册一个动态配置并返回并发安全的值容器。

	参数:
	  - dataId: Nacos 配置 Data ID（配置必须已存在，否则 Fatal）
	  - decoder: 内容解码器，必传；nil 直接 Fatal
	  - callbacks: 可选的变更回调，首次监听送达与每次热更新时都会触发

	返回:
	  - *Value[V]: 值容器，通过 Get() 读取最新配置，Nacos 侧变更会自动热更新

	语义:
	  - 同步拉取一次当前值作为容器初值（拉取失败 Fatal，fail-fast）
	  - 注册长轮询监听，变更时解码并替换容器内值
	  - 注意：热启动（本地有 SDK 缓存）时监听不会为初值触发回调，
	    因为 SDK 以缓存内容为基准、服务端判定无变化——初值只来自同步拉取
*/
func Register[V any](dataId string, decoder Decoder[V], callbacks ...Callback[V]) *Value[V] {
	if decoder == nil {
		logger.Logger.Fatal().Msg("decoder is nil")
	}
	config, err := configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		logger.Logger.Fatal().Msgf("failed to get config %s: %v", dataId, err)
	}
	value := NewValue(decoder(config))
	err = configClient.ListenConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
		OnChange: func(namespace, group, dataId, data string) {
			v := decoder(data)
			value.set(v)
			for _, callback := range callbacks {
				callback(v)
			}
			logger.Logger.Info().Msgf("dynamic: config changed %s/%s/%s", namespace, group, dataId)
		},
	})
	if err != nil {
		logger.Logger.Fatal().Msgf("dynamic: listen %s failed: %v", dataId, err)
	}
	return value
}
