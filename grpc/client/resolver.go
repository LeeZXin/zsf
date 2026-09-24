package client

import (
	"fmt"
	"net/netip"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/discovery/nacos"
	"github.com/LeeZXin/zsf/services/lb"
	"github.com/LeeZXin/zsf/utils/jsonutil"
	"github.com/LeeZXin/zsf/utils/listutil"

	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	r discovery.Resolver
}

func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()
	r := &customResolver{
		serviceName: serviceName,
		cc:          cc,
		r:           b.r,
	}
	r.Start()
	return r, nil
}

func newResolverBuilder() resolver.Builder {
	return &resolverBuilder{r: nacos.NewResolver()}
}

func (b *resolverBuilder) Scheme() string {
	return ""
}

type customResolver struct {
	serviceName string
	cc          resolver.ClientConn
	r           discovery.Resolver
}

func (r *customResolver) Start() {
	_, err := netip.ParseAddrPort(r.serviceName)
	if err == nil {
		err = r.cc.UpdateState(resolver.State{
			Addresses: []resolver.Address{{
				Addr: r.serviceName,
			}},
		})
		if err != nil {
			logger.Logger.Error().Msgf("Error updating resolver state: %v", err)
		}
		return
	}
	r.r.Watch(r.serviceName, func(servers []lb.Server) {
		servers = listutil.FilterNe(servers, func(server lb.Server) bool {
			return server.Protocol == rpc.GrpcProtocol
		})
		logger.Logger.Info().Msgf("grpc watch %s changed: %v", r.serviceName, jsonutil.MarshalStringIgnoreErr(servers))
		r.updateState(servers)
	})
}

func (r *customResolver) updateState(servers []lb.Server) {
	err := r.cc.UpdateState(resolver.State{
		Addresses: listutil.MapNe(servers, func(t lb.Server) resolver.Address {
			return resolver.Address{
				Addr: fmt.Sprintf("%s:%d", t.Host, t.Port),
			}
		}),
	})
	if err != nil {
		logger.Logger.Error().Msgf("failed to update state of service %s: %v", r.serviceName, err)
	}
}

func (r *customResolver) ResolveNow(resolver.ResolveNowOptions) {

}

func (r *customResolver) Close() {

}
