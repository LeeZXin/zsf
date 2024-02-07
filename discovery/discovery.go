package discovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	_ "github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/rpcheader"
	"strconv"
)

var (
	discoveryType = static.GetString("discovery.type")
	lbPolicy      = static.GetString("discovery.lbPolicy")

	ServiceNotFound = errors.New("service not found")

	discoveryMap = map[string]Discovery{
		StaticDiscoveryType: new(staticDiscovery),
		EtcdDiscoveryType:   new(etcdDiscovery),
	}

	discoveryImpl Discovery
)

type Type string

const (
	StaticDiscoveryType Type = "static"
	EtcdDiscoveryType   Type = "etcd"
)

func init() {
	if discoveryType == "" {
		discoveryType = StaticDiscoveryType
	}
	if lbPolicy == "" {
		lbPolicy = selector.RoundRobinPolicy
	}
	discoveryImpl = discoveryMap[discoveryType]
	if discoveryImpl == nil {
		discoveryImpl = discoveryMap[StaticDiscoveryType]
	}
	discoveryImpl.Init()
}

type Discovery interface {
	Init()
	GetDiscoveryType() Type
	GetServiceInfo(string) ([]ServiceAddr, error)
	PickOne(context.Context, string) (ServiceAddr, error)
	OnAddrChange(string, ServiceChangeFunc)
}

type ServiceChangeFunc func([]ServiceAddr)

// ServiceAddr 服务信息
type ServiceAddr struct {
	InstanceId string `json:"instanceId"`
	Name       string `json:"name"`
	Addr       string `json:"addr"`
	Port       int    `json:"port"`
	Weight     int    `json:"weight"`
	Version    string `json:"version"`
}

func (s *ServiceAddr) IsSameAs(s2 *ServiceAddr) bool {
	if s2 == nil {
		return false
	}
	return s.Name == s2.Name &&
		s.Addr == s2.Addr &&
		s.Port == s2.Port &&
		s.Weight == s2.Weight &&
		s.Version == s2.Version
}

func GetDiscovery() Discovery {
	return discoveryImpl
}

func OnAddrChange(name string, fn ServiceChangeFunc) {
	discoveryImpl.OnAddrChange(name, fn)
}

func findSelector(ctx context.Context, selectorMap map[string]selector.Selector[ServiceAddr]) selector.Selector[ServiceAddr] {
	version := rpcheader.GetHeaders(ctx).Get(rpcheader.ApiVersion)
	if version == "" {
		version = env.GetVersion()
	}
	hit, ok := selectorMap[version]
	if !ok {
		hit = selectorMap[common.DefaultVersion]
	}
	return hit
}

func convertToSelector(nodesMap map[string][]selector.Node[ServiceAddr], lbPolicy string) map[string]selector.Selector[ServiceAddr] {
	ret := make(map[string]selector.Selector[ServiceAddr], len(nodesMap))
	fn, ok := selector.FindNewSelectorFunc[ServiceAddr](lbPolicy)
	if !ok {
		fn = selector.NewRoundRobinSelector[ServiceAddr]
	}
	for version, nodes := range nodesMap {
		ret[version] = fn(nodes)
	}
	return ret
}

// convertMultiVersionNodes 返回多版本地址信息
func convertMultiVersionNodes(addrs []ServiceAddr) map[string][]selector.Node[ServiceAddr] {
	ret := make(map[string][]selector.Node[ServiceAddr], 8)
	//默认版本节点先初始化
	ret[common.DefaultVersion] = make([]selector.Node[ServiceAddr], 0)
	for i, addr := range addrs {
		if addr.Version == "" {
			addr.Version = common.DefaultVersion
		}
		n := selector.Node[ServiceAddr]{
			Id:     strconv.Itoa(i),
			Weight: addr.Weight,
			Data:   addr,
		}
		ns, ok := ret[addr.Version]
		if ok {
			ret[addr.Version] = append(ns, n)
		} else {
			ret[addr.Version] = append(make([]selector.Node[ServiceAddr], 0), n)
		}
		if addr.Version != common.DefaultVersion {
			ret[common.DefaultVersion] = append(ret[common.DefaultVersion], n)
		}
	}
	return ret
}

func PickOneHost(ctx context.Context, serviceName string) (string, error) {
	one, err := discoveryImpl.PickOne(ctx, serviceName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", one.Addr, one.Port), nil
}

func compareAddrs(xs, ys []ServiceAddr) bool {
	if len(xs) != len(ys) {
		return false
	}
	for _, x := range xs {
		find := false
		for _, y := range ys {
			if x.IsSameAs(&y) {
				find = true
				break
			}
		}
		if !find {
			return false
		}
	}
	return true
}
