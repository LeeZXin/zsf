package grpcclient

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/cache"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/grpc/client/balancer"
	"github.com/LeeZXin/zsf/grpc/debug"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/selector"
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
	channelMap  *cache.MapCache
	ipRegexp, _ = regexp.Compile("^\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}:\\d+$")
	//节点变更watcher
	wa *watcher
	//全局拦截器
	globalClientUnaryInterceptors = make([]grpc.UnaryClientInterceptor, 0)
	//全局拦截器
	globalClientStreamInterceptors = make([]grpc.StreamClientInterceptor, 0)
	//锁
	globalClientInterceptorsMu = sync.Mutex{}
)

func init() {
	//默认三种拦截器
	RegisterGlobalUnaryClientInterceptor(headerClientUnaryInterceptor(), promClientUnaryInterceptor(), skywalkingUnaryInterceptor())
	RegisterGlobalStreamClientInterceptor(headerStreamInterceptor(), promStreamInterceptor(), skywalkingStreamInterceptor())
	//加载缓存
	channelMap = &cache.MapCache{
		SupplierWithKey: func(serviceName string) (any, error) {
			// 选择负载均衡策略
			lbPolicy := property.GetString("grpc.lbPolicy")
			c, ok := loadBalancingPolicy[lbPolicy]
			if !ok {
				c = loadBalancingPolicy[selector.RoundRobinPolicy]
			}
			globalClientInterceptorsMu.Lock()
			opts := []grpc.DialOption{
				grpc.WithDefaultServiceConfig(c),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Timeout: 5 * time.Minute,
				}),
				grpc.WithChainUnaryInterceptor(
					globalClientUnaryInterceptors...,
				),
			}
			globalClientInterceptorsMu.Unlock()
			ch, err := grpc.DialContext(context.Background(), serviceName, opts...)
			if err != nil {
				return nil, err
			}
			return ch, nil
		},
	}
	//每十秒更新
	wa = newWatcher(10 * time.Second)
	wa.Start()
	//关闭所有的连接
	quit.AddShutdownHook(func() {
		wa.Shutdown()
	})
	//开启grpc debug
	if property.GetBool("grpc.debug") {
		debug.StartGrpcDebug()
	}
}

type targetResolver struct {
	target resolver.Target
	cc     resolver.ClientConn
	opts   resolver.BuildOptions
}

func (*targetResolver) ResolveNow(options resolver.ResolveNowOptions) {

}

func (*targetResolver) Close() {
}

func getResolverState(addresses []discovery.ServiceAddr) resolver.State {
	if addresses == nil {
		return resolver.State{Addresses: []resolver.Address{}}
	}
	addrs := make([]resolver.Address, len(addresses))
	for i, item := range addresses {
		addrs[i] = resolver.Address{
			Addr: fmt.Sprintf("%s:%d", item.Addr, item.Port),
			Attributes: attributes.New(balancer.AttrKey, balancer.Attr{
				Weight:  item.Weight,
				Version: item.Version,
			}),
		}
	}
	return resolver.State{Addresses: addrs}
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
	if isIp(serviceName) {
		_ = cc.UpdateState(resolver.State{
			Addresses: []resolver.Address{
				{Addr: serviceName},
			},
		})
	} else {
		// 注册服务变动回调 返回注册时的服务列表
		wa.Register(serviceName, func(addrs []discovery.ServiceAddr) {
			logger.Logger.Info("update addr:", serviceName, addrs)
			_ = cc.UpdateState(getResolverState(addrs))
		})
	}
	return r, nil
}

// 初始化
func init() {
	resolver.Register(&targetResolverBuilder{})
}

func isIp(name string) bool {
	return ipRegexp.MatchString(name)
}

// Dial 构建channel
// 优先从缓存里取
func Dial(name string) (*grpc.ClientConn, error) {
	conn, err := channelMap.Get(name)
	if err != nil {
		return nil, err
	}
	return conn.(*grpc.ClientConn), nil
}

// RegisterGlobalUnaryClientInterceptor 注册全局一元拦截器
func RegisterGlobalUnaryClientInterceptor(is ...grpc.UnaryClientInterceptor) {
	if is == nil || len(is) == 0 {
		return
	}
	globalClientInterceptorsMu.Lock()
	defer globalClientInterceptorsMu.Unlock()
	globalClientUnaryInterceptors = append(globalClientUnaryInterceptors, is...)
}

// RegisterGlobalStreamClientInterceptor 注册全局流拦截器
func RegisterGlobalStreamClientInterceptor(is ...grpc.StreamClientInterceptor) {
	if is == nil || len(is) == 0 {
		return
	}
	globalClientInterceptorsMu.Lock()
	defer globalClientInterceptorsMu.Unlock()
	globalClientStreamInterceptors = append(globalClientStreamInterceptors, is...)
}
