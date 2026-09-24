// Package client 提供 gRPC 客户端连接管理：按目标名缓存复用连接，并统一注入
// rpc 请求头透传、超时控制与 prometheus 埋点拦截器。
//
// 目标名解析规则由同包 resolver 实现：目标能被解析为 host:port 时直连该地址；
// 否则视为服务名，通过 Nacos 服务发现解析实例并按 roundrobin 负载均衡。
//
// 注意：
//   - grpc.NewClient 为懒建连，Dial 返回后连接可能仍在建立中，首个请求
//     会在连接就绪前等待（由 gRPC 内部重试/等待机制处理）
//   - 连接生命周期由框架统一管理：进程退出时高优先级 shutdown hook 统一
//     关闭，调用方不要自行 Close（从缓存取出的连接会被复用）
//   - 连接按 name 缓存，缓存的连接数随 name 数量线性增长，没有回收机制
package client

import (
	"fmt"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/container/hashmap"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
)

var (
	connMap = hashmap.NewStringSyncMap[*grpc.ClientConn]()
)

func init() {
	start.AddInit(func() {
		quit.AddHighPriorityShutdownHook(func() {
			connMap.Range(func(_ string, conn *grpc.ClientConn) bool {
				conn.Close()
				return true
			})
		})
		if static.GetBool("grpc.client.discovery.enable") {
			resolver.Register(newResolverBuilder())
		}
	}, -5)
}

// Dial 按 name 获取 gRPC 连接（线程安全），相同 name 的连接被缓存复用。
// 并发首次调用可能临时创建多个连接，但只有最先存进缓存的被保留，其余立即
// 关闭（grpc.NewClient 为懒建连，此时尚未发起任何网络行为，Close 是安全的）；
// name 为服务名或 host:port（解析规则见包注释）。
// 连接生命周期由框架管理（进程退出时统一关闭），调用方不要自行 Close。
func Dial(name string) grpc.ClientConnInterface {
	if v, ok := connMap.Load(name); ok {
		return v
	}
	conn, err := grpc.NewClient(
		name,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			promUnaryInterceptor,
			headerUnaryInterceptor,
			timeoutUnaryInterceptor,
		),
		grpc.WithChainStreamInterceptor(
			headerStreamInterceptor,
		),
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, roundrobin.Name)),
	)
	if err != nil {
		logger.Logger.Fatal().Msgf("failed to dial grpc client %s: %v", name, err)
	}
	actual, loaded := connMap.LoadOrStore(name, conn)
	if loaded {
		conn.Close()
	}
	return actual
}
