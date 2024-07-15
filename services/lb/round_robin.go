package lb

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf-utils/listutil"
)

var (
	ServerNotFound = errors.New("server not found")
)

type roundRobinLoadBalancer struct {
	allServers []Server
	incr       uint64
}

func (r *roundRobinLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
	r.allServers = listutil.Shuffle(servers)
	r.incr = 0
}

func (r *roundRobinLoadBalancer) GetServers() []Server {
	return r.allServers
}

func (r *roundRobinLoadBalancer) ChooseServer(_ context.Context) (Server, error) {
	if len(r.allServers) == 0 {
		return Server{}, ServerNotFound
	}
	ret := r.allServers[r.incr%uint64(len(r.allServers))]
	r.incr++
	return ret, nil
}
