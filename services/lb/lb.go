// Package lb 定义服务发现/负载均衡共享的数据模型与工具。
//
// Server 描述一个可寻址的服务实例（服务名 + 地址 + 协议），由
// services/discovery（当前为 Nacos）产出，供 grpc 客户端 resolver、
// 注册中心与业务侧按服务名寻址使用；本包不依赖具体注册中心实现。
package lb

import (
	"errors"
	"math/rand/v2"

	"github.com/LeeZXin/zsf/rpc"
)

// ServerNotFound 表示目标服务没有可用（健康）实例。
var (
	ServerNotFound = errors.New("server not found")
)

// Server 服务实例信息：Name 为服务名（application.name），Host/Port 为实例
// 监听地址，Protocol 为该实例提供的协议（http/grpc）。同一实例可能按协议
// 注册两条记录（见 services/registry），消费方需按协议过滤。
type Server struct {
	Host     string       `json:"host"`
	Port     int          `json:"port"`
	Protocol rpc.Protocol `json:"protocol"`
}

// IsSameAs 判断两个服务实例是否完全一致（四个字段全等，与比较顺序无关）。
func (s *Server) IsSameAs(s2 Server) bool {
	return s.Host == s2.Host &&
		s.Port == s2.Port &&
		s.Protocol == s2.Protocol
}

// CompareServers 有序比较两个实例列表是否一致：长度相同且同位置实例
// 逐一相等才返回 true。注意结果依赖实例顺序，仅适用于顺序有意义的场景。
func CompareServers(s1, s2 []Server) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if !s1[i].IsSameAs(s2[i]) {
			return false
		}
	}
	return true
}

// Callback 服务实例变更回调函数，参数为最新全量实例快照
// （见 services/discovery.Resolver.Watch）。
type Callback func([]Server)

// ChooseRandomServer 从实例列表中随机选一个返回。
// 列表为空时返回零值 Server{} 且不报错——调用方需自行判断
// len(servers) == 0，或依赖 SelectOne 返回的 ServerNotFound。
func ChooseRandomServer(servers []Server) Server {
	l := len(servers)
	if l == 0 {
		return Server{}
	}
	if l == 1 {
		return servers[0]
	}
	return servers[rand.IntN(l)]
}
