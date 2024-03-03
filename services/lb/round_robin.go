package lb

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf-utils/listutil"
	"sync"
)

var (
	ServerNotFound = errors.New("server not found")
)

type RoundRobinLoadBalancer struct {
	smu        sync.RWMutex
	allServers []Server
	incr       uint64
}

func (r *RoundRobinLoadBalancer) SetServers(servers []Server) {
	if len(servers) == 0 {
		return
	}
	r.smu.Lock()
	defer r.smu.Unlock()
	r.allServers = listutil.Shuffle(servers)
	r.incr = 0
}

func (r *RoundRobinLoadBalancer) ChooseServer(_ context.Context) (Server, error) {
	r.smu.RLock()
	defer r.smu.RUnlock()
	if len(r.allServers) == 0 {
		return Server{}, ServerNotFound
	}
	ret := r.allServers[r.incr%uint64(len(r.allServers))]
	r.incr++
	return ret, nil
}
