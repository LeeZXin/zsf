// Package nacos 提供基于 Nacos naming 的服务发现实现（services/discovery
// 的 Resolver 接口落地），供 grpc 客户端 resolver 与按服务名寻址的业务使用。
//
// 与注册侧（services/registry）约定一致：
//   - 服务名 = application.name；实例 metadata 携带 protocol 标记，
//     发现结果经 lb.Server.Protocol 透出，由消费方按协议过滤
//   - Resolve/SelectOne 数据来自 SDK 本地缓存（未命中时 SDK 内部触发
//     一次订阅），不产生逐次服务端请求
//   - Watch 依赖 SDK 原生 Subscribe（naming-push 推送）；同一服务的服务端
//     订阅只建立一次（SDK 不去重，重复 Subscribe 会重复发订阅请求），
//     重复 Watch 仅追加本地回调
package nacos

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/lb"
	"github.com/LeeZXin/zsf/utils/jsonutil"
	"github.com/LeeZXin/zsf/utils/listutil"
	"github.com/LeeZXin/zsf/utils/nacosutil"

	"context"
	"slices"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

/*
Resolver 基于 Nacos naming 的服务发现器。

与 registry 侧的约定对应：

  - 服务名 = 目标服务的 application.name，即注册侧 Register 用的名字
  - 同一服务下混合 http/grpc 两种协议的实例（registry 按协议各注册一条，
    metadata 带 protocol 标记），协议信息随 lb.Server.Protocol 透出，
    由消费方自行过滤
  - Watch 使用 SDK 原生 Subscribe（服务端变更时 naming-push 秒级推送）；
    **同一 name 的服务端订阅只发一次**（SDK 不做去重，重复 Subscribe 会
    重复向服务端发订阅请求），后续 Watch 仅追加本地回调，推送到达时
    逐个分发
  - Subscribe 给的是全量实例，回调前按 usable 过滤（Healthy && Enable &&
    Weight>0），与 SelectInstances(HealthyOnly=true) 一致，避免 Disable /
    权重为 0 的实例仍进入 gRPC 连接池
*/
type Resolver struct {
	client naming_client.INamingClient
	group  string

	mu        sync.Mutex
	callbacks map[string][]lb.Callback
}

// NewResolver 创建 Nacos 服务发现器。
// 内置 1s 等待 gRPC 建连（SDK 异步建连，STARTING 窗口实测 <300ms 常见、极端 ~2s）。
func NewResolver() *Resolver {
	group := nacosutil.GetGroup("nacos.naming.group")
	client, err := clients.NewNamingClient(nacosutil.MustNewClientParam())
	if err != nil {
		logger.Logger.Fatal().Msgf("init naming client failed: %v", err)
	}
	quit.AddFinalShutdownHook(client.CloseClient)
	time.Sleep(time.Second)
	return &Resolver{
		client:    client,
		group:     group,
		callbacks: make(map[string][]lb.Callback),
	}
}

// Resolve 查询指定服务在当前协议下的健康实例列表。
func (r *Resolver) Resolve(_ context.Context, names ...string) ([]lb.Server, error) {
	if len(names) == 0 {
		return nil, lb.ServerNotFound
	}
	ret := make([]lb.Server, 0)
	for _, name := range names {
		instances, err := r.client.SelectInstances(vo.SelectInstancesParam{
			ServiceName: name,
			GroupName:   r.group,
			HealthyOnly: true,
		})
		if err != nil {
			return nil, err
		}
		servers, _ := listutil.NewPipe(instances).FilterNe(usable).MapNe(toServer).Data()
		if len(servers) > 0 {
			ret = append(ret, servers...)
		}
	}
	return ret, nil
}

// SelectOne 从 Nacos 客户端缓存中随机挑选一个指定协议的健康实例。
// 数据来自本地缓存（未命中时 SDK 内部触发一次订阅），过滤与挑选
// 在本地完成，不产生逐次服务端请求。
func (r *Resolver) SelectOne(_ context.Context, name string, protocol rpc.Protocol) (lb.Server, error) {
	instances, err := r.client.SelectInstances(vo.SelectInstancesParam{
		ServiceName: name,
		GroupName:   r.group,
		HealthyOnly: true,
	})
	if err != nil {
		return lb.Server{}, err
	}
	servers, _ := listutil.NewPipe(instances).FilterNe(func(t model.Instance) bool {
		return rpc.Protocol(t.Metadata["protocol"]) == protocol && usable(t)
	}).MapNe(toServer).Data()
	if len(servers) == 0 {
		return lb.Server{}, lb.ServerNotFound
	}
	return lb.ChooseRandomServer(servers), nil
}

// Watch 用 SDK 原生 Subscribe 监听指定服务的实例变化（服务端 push）。
// 同一 name 多次 Watch 时服务端订阅只发一次，后续调用仅追加本地回调。
// 回调收到的是可用实例（见 usable），与 Resolve/SelectOne 同一套过滤。
func (r *Resolver) Watch(name string, callback lb.Callback) {
	servers, err := r.Resolve(nil, name)
	if err != nil {
		logger.Logger.Error().Msgf("resolver resolve %s failed: %v", name, err)
	} else {
		callback(servers)
	}
	r.mu.Lock()
	subscribed := len(r.callbacks[name]) > 0
	r.callbacks[name] = append(r.callbacks[name], callback)
	r.mu.Unlock()

	if subscribed {
		return
	}
	err = r.client.Subscribe(&vo.SubscribeParam{
		ServiceName: name,
		GroupName:   r.group,
		SubscribeCallback: func(instances []model.Instance, err error) {
			if err != nil {
				logger.Logger.Error().Msgf("discovery: subscribe %s failed: %v", name, err)
				return
			}
			lbServers, _ := listutil.NewPipe(instances).FilterNe(usable).MapNe(toServer).Data()
			logger.Logger.Info().Msgf("discovery: subscribe %s changed: %s", name, jsonutil.MarshalStringIgnoreErr(lbServers))
			// 拷贝回调列表后在锁外分发，回调内再次 Watch 不会死锁
			r.mu.Lock()
			callbacks := slices.Clone(r.callbacks[name])
			r.mu.Unlock()
			for _, cb := range callbacks {
				cb(lbServers)
			}
		},
	})
	if err != nil {
		logger.Logger.Error().Msgf("discovery: subscribe %s failed: %v", name, err)
	}
}

// usable 与 SDK SelectInstances(HealthyOnly=true) 一致：健康、可访问、权重>0。
// Watch 的 Subscribe 推送是全量快照，必须在本地套同一套过滤，否则控制台
// Disable 或权重打 0 的实例仍会进入 gRPC 连接池。
func usable(t model.Instance) bool {
	return t.Healthy && t.Enable && t.Weight > 0
}

func toServer(t model.Instance) lb.Server {
	return lb.Server{
		Host:     t.Ip,
		Port:     int(t.Port),
		Protocol: rpc.Protocol(t.Metadata["protocol"]),
	}
}
