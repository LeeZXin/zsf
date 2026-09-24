// Package discovery 定义服务发现抽象（Resolver 接口）：按服务名解析实例
// 列表/挑选单实例、以及订阅实例变更。
// 当前实现：services/discovery/nacos（基于 Nacos naming）；消费方包括
// grpc 客户端的自定义 resolver 与按服务名寻址的业务代码。
package discovery

import (
	"context"

	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/lb"
)

// Resolver 服务发现接口：
//   - Resolve 查询指定服务（可传多个服务名）的健康实例列表，返回的实例
//     混含各协议，由调用方按 lb.Server.Protocol 过滤
//   - SelectOne 按协议挑选一个健康实例；无匹配实例返回 lb.ServerNotFound
//   - Watch 订阅指定服务的实例变更，回调收到可用实例快照（健康、Enable、
//     权重>0，与 Resolve/SelectOne 一致）；可对同一 name 多次调用叠加回调
//     （实现见 nacos.Resolver）
//
// 实现要求并发安全：Resolve/SelectOne 与 Watch 可并发调用。
type Resolver interface {
	Resolve(context.Context, ...string) ([]lb.Server, error)
	SelectOne(context.Context, string, rpc.Protocol) (lb.Server, error)
	Watch(string, lb.Callback)
}
