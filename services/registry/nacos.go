// Package registry 提供服务注册能力（当前实现：Nacos naming 自注册）。
package registry

import (
	"time"

	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/utils/nacosutil"

	"fmt"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Registrar 服务注册器：按协议把本服务实例注册/注销到注册中心。
type Registrar interface {
	Register() error
	Deregister()
}

// NewRegistrarFunc 注册器工厂函数：给定协议与监听端口，创建对应的
// Registrar（当前实现为 Nacos 注册器，见 NewNacosRegistrar）。
// 由 grpc/server 与 http/server 的 WithNewRegistrarFunc 接入生命周期。
type NewRegistrarFunc func(rpc.Protocol, int) (Registrar, error)

/*
nacosRegistrar 基于 Nacos naming 的服务注册器，一个实例对应一个协议/端口。

设计约定（与 property/dynamic 包共用同一套 nacos.* 配置）：

  - 服务名：application.name（instance.ApplicationName），发现侧保持一致
  - 端口：由调用方传入（构造参数），实例 metadata 带 protocol 标记
    供发现侧按协议过滤
  - 注册 IP：env.LocalIP（本机可达地址，多机部署以框架的 IP 探测为准）
  - 临时实例（Ephemeral）：心跳由 SDK 自动维护；断线重连后 SDK 会按
    redo 缓存自动补注册；退出时由调用方决定何时 Deregister——
    进程退出未显式注销时，服务端按心跳超时摘除（15s 判不健康、30s 删除）
  - **每协议各持独立 client**：实测 2.x Go SDK 的同一个 client 对同一服务
    逐条注册多个实例时，Nacos 3.x 服务端只保留最后一个（同 client 实例
    集合是替换语义）；按协议隔离 client 后互不覆盖，天然规避该坑
  - SDK 日志桥接依赖 dynamic 包（SetLogger 是全局的）；未引入 dynamic 时
    SDK 日志落自己的文件，不影响功能
*/
type nacosRegistrar struct {
	client   naming_client.INamingClient
	protocol rpc.Protocol
	port     uint64
	group    string
}

// NewNacosRegistrar 创建指定协议/端口的注册器并初始化独立 naming client。
// 连接等待在 Register 内完成（见 Register 注释）。
var NewNacosRegistrar NewRegistrarFunc = func(protocol rpc.Protocol, port int) (Registrar, error) {
	group := nacosutil.GetGroup("nacos.naming.group")
	client, err := clients.NewNamingClient(nacosutil.MustNewClientParam())
	if err != nil {
		return nil, fmt.Errorf("init naming client failed: %v", err)
	}
	quit.AddFinalShutdownHook(client.CloseClient)
	return &nacosRegistrar{
		client:   client,
		protocol: protocol,
		port:     uint64(port),
		group:    group,
	}, nil
}

/*
Register 注册本服务实例。

	内部先等待 1s 让 gRPC 建连（SDK 异步建连，STARTING 窗口实测
	<300ms 常见、极端 ~2s；未就绪时 RegisterInstance 会快速失败），
	然后注册临时实例；err 与 bool 分别表示通信失败与服务端拒绝。
	不自动挂注销钩子：退出时的显式注销由调用方通过 Deregister 控制，
	未调用时依赖服务端心跳超时摘除。
*/
func (n *nacosRegistrar) Register() error {
	time.Sleep(time.Second)
	ok, err := n.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          env.LocalIP,
		Port:        n.port,
		ServiceName: instance.ApplicationName,
		GroupName:   n.group,
		Weight:      1,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata:    map[string]string{"protocol": string(n.protocol)},
	})
	if err != nil {
		return fmt.Errorf("register instance failed: %v", err)
	}
	if !ok {
		return fmt.Errorf("register instance failed")
	}
	logger.Logger.Info().Msgf("registry: registered %s(%s:%d) as %s",
		n.protocol, env.LocalIP, n.port, n.group)
	return nil
}

// Deregister 显式注销本实例（幂等；Ephemeral 与注册时保持一致，
// 否则服务端在错误的实例存储中查找导致注销无效）。失败仅记录日志：
// 心跳超时兜底最终会摘除，此处不放大错误。
func (n *nacosRegistrar) Deregister() {
	_, err := n.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          env.LocalIP,
		Port:        n.port,
		ServiceName: instance.ApplicationName,
		GroupName:   n.group,
		Ephemeral:   true,
	})
	if err != nil {
		logger.Logger.Error().Msgf("deregister instance failed: %v", err)
	}
}
