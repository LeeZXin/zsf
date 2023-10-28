package discovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf-utils/listutil"
	"github.com/LeeZXin/zsf-utils/localcache"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/rpcheader"
	"strconv"
	"time"
)

var (
	defaultDiscovery = newDefaultDiscovery()
)

type DefaultDiscovery struct {
	lbPolicy string
	//多版本路由
	targetCache   localcache.ExpireCache[map[string]selector.Selector[string]]
	discoveryType string
}

func newDefaultDiscovery() *DefaultDiscovery {
	ret := &DefaultDiscovery{
		lbPolicy: static.GetString("discovery.default.lbPolicy"),
	}
	expireDuration := static.GetInt64("discovery.default.expireDuration")
	if expireDuration <= 0 {
		expireDuration = 10
	}
	entry, _ := localcache.NewLocalCache[map[string]selector.Selector[string]](func(_ context.Context, serviceName string) (map[string]selector.Selector[string], error) {
		//consul拿服务信息
		nodesMap, err := serviceMultiVersionNodes(serviceName)
		if err != nil {
			return nil, err
		}
		return convert2Selector(nodesMap), nil
	}, time.Duration(expireDuration)*time.Second)
	ret.targetCache = entry
	return ret
}

func (c *DefaultDiscovery) SelectOne(ctx context.Context, serviceName string) (string, error) {
	nodesMap, err := c.targetCache.LoadData(ctx, serviceName)
	if err != nil {
		return "", err
	}
	ret, err := getFromCache(ctx, nodesMap)
	if err != nil {
		return "", err
	}
	return ret.Data, nil
}

func (c *DefaultDiscovery) SelectMulti(ctx context.Context, serviceName string) ([]string, error) {
	nodesMap, err := c.targetCache.LoadData(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	s := getSelector(ctx, nodesMap)
	return listutil.Map(s.GetNodes(), func(t selector.Node[string]) (string, error) {
		return t.Data, nil
	})
}

func getFromCache(ctx context.Context, selectorMap map[string]selector.Selector[string]) (selector.Node[string], error) {
	return getSelector(ctx, selectorMap).Select()
}

func getSelector(ctx context.Context, selectorMap map[string]selector.Selector[string]) selector.Selector[string] {
	version := rpcheader.GetHeaders(ctx).Get(rpcheader.ApiVersion)
	if version == "" {
		version = cmd.GetVersion()
	}
	hit, ok := selectorMap[version]
	if !ok {
		hit = selectorMap[common.DefaultVersion]
	}
	return hit
}

func convert2Selector(nodesMap map[string][]selector.Node[string]) map[string]selector.Selector[string] {
	ret := make(map[string]selector.Selector[string], len(nodesMap))
	fn, ok := selector.FindNewSelectorFunc[string](static.GetString("discovery.default.lbPolicy"))
	if !ok {
		fn = selector.NewRoundRobinSelector[string]
	}
	for version, nodes := range nodesMap {
		ret[version] = fn(nodes)
	}
	return ret
}

func serviceMultiVersionNodes(serviceName string) (map[string][]selector.Node[string], error) {
	info, err := GetServiceInfoByDiscoveryType(serviceName, static.GetString("discovery.default.type"))
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, errors.New("can not find ip address")
	}
	res := make(map[string][]selector.Node[string])
	//默认版本节点先初始化
	res[common.DefaultVersion] = make([]selector.Node[string], 0)
	for i, item := range info {
		n := selector.Node[string]{
			Id:     strconv.Itoa(i),
			Weight: item.Weight,
			Data:   fmt.Sprintf("%s:%d", item.Addr, item.Port),
		}
		if item.Version == "" {
			item.Version = common.DefaultVersion
		}
		ns, ok := res[item.Version]
		if ok {
			res[item.Version] = append(ns, n)
		} else {
			res[item.Version] = append(make([]selector.Node[string], 0), n)
		}
		if item.Version != common.DefaultVersion {
			res[common.DefaultVersion] = append(res[common.DefaultVersion], n)
		}
	}
	return res, nil
}

func SelectOneIpPort(ctx context.Context, serviceName string) (string, error) {
	return defaultDiscovery.SelectOne(ctx, serviceName)
}

func SelectMultiIpPorts(ctx context.Context, serviceName string) ([]string, error) {
	return defaultDiscovery.SelectMulti(ctx, serviceName)
}
