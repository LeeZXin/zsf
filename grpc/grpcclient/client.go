package grpcclient

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/maputil"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/grpc/debug"
	"github.com/LeeZXin/zsf/grpc/grpcclient/balancer"
	"github.com/LeeZXin/zsf/property/static"
	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/resolver"
	"regexp"
	"sync"
	"time"
)

// grpc client封装
// 封装负载均衡策略
// 可实现根据服务版本号路由，可用于灰度发布
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

var (
	// 负载均衡策略 具体查看balancer包
	loadBalancingPolicy = map[string]string{
		selector.RoundRobinPolicy: `{
			"loadBalancingPolicy": "round_robin"
		}`,
		selector.WeightedRoundRobinPolicy: `{
			"loadBalancingPolicy": "weighted_round_robin"
		}`,
	}
	connCache   = maputil.NewConcurrentMap[string, *grpc.ClientConn](nil)
	ipRegexp, _ = regexp.Compile("^\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}:\\d+$")
	initOnce    = sync.Once{}
)

func initGrpc() {
	//默认三种拦截器
	AppendUnaryInterceptors(
		headerClientUnaryInterceptor(),
		promClientUnaryInterceptor(),
		skywalkingUnaryInterceptor(),
	)
	AppendStreamInterceptors(
		headerStreamInterceptor(),
		promStreamInterceptor(),
		skywalkingStreamInterceptor(),
	)
	//开启grpc debug
	if static.GetBool("grpc.debug") {
		debug.StartGrpcDebug()
	}
	//关闭grpc channel
	quit.AddShutdownHook(func() {
		connCache.Range(func(_ string, conn *grpc.ClientConn) bool {
			conn.Close()
			return true
		})
	})
}

type targetResolver struct {
	target resolver.Target
	cc     resolver.ClientConn
	opts   resolver.BuildOptions
}

func (*targetResolver) ResolveNow(_ resolver.ResolveNowOptions) {
}

func (*targetResolver) Close() {
}

func getResolverState(addresses []discovery.ServiceAddr) resolver.State {
	if addresses == nil {
		return resolver.State{Addresses: []resolver.Address{}}
	}
	ret := make([]resolver.Address, 0, len(addresses))
	for _, item := range addresses {
		ret = append(ret, resolver.Address{
			Addr: fmt.Sprintf("%s:%d", item.Addr, item.Port),
			Attributes: attributes.New(balancer.AttrKey, balancer.Attr{
				Weight:  item.Weight,
				Version: item.Version,
			}),
		})
	}
	return resolver.State{Addresses: ret}
}

type targetResolverBuilder struct{}

func (*targetResolverBuilder) Scheme() string {
	return ""
}

func (*targetResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &targetResolver{
		target: target,
		cc:     cc,
		opts:   opts,
	}
	serviceName := target.Endpoint()
	// 如果是ip，则无需服务发现
	if ipRegexp.MatchString(serviceName) {
		_ = cc.UpdateState(resolver.State{
			Addresses: []resolver.Address{
				{Addr: serviceName},
			},
		})
	} else {
		// 注册服务变动回调 返回注册时的服务列表
		discovery.OnAddrChange(serviceName, func(addrs []discovery.ServiceAddr) {
			_ = cc.UpdateState(getResolverState(addrs))
		})
	}
	return r, nil
}

// 初始化
func init() {
	resolver.Register(&targetResolverBuilder{})
}

// Dial 构建channel
// 优先从缓存里取
func Dial(serviceName string) (*grpc.ClientConn, error) {
	return connCache.LoadOrStoreWithLoader(serviceName, func() (*grpc.ClientConn, error) {
		initOnce.Do(func() {
			initGrpc()
		})
		// 选择负载均衡策略
		lbPolicy := static.GetString("grpc.lbPolicy")
		lbConfig, ok := loadBalancingPolicy[lbPolicy]
		if !ok {
			lbConfig = loadBalancingPolicy[selector.RoundRobinPolicy]
		}
		opts := []grpc.DialOption{
			grpc.WithDefaultServiceConfig(lbConfig),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Timeout: 5 * time.Minute,
			}),
			grpc.WithChainUnaryInterceptor(
				getUnaryInterceptors()...,
			),
			grpc.WithChainStreamInterceptor(
				getStreamInterceptors()...,
			),
		}
		conn, err := grpc.DialContext(context.Background(), serviceName, opts...)
		if err != nil {
			return nil, err
		}
		return conn, nil
	})
}
