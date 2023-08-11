package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/cache"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/selector"
	"strconv"
	"time"
)

// 缓存服务ip的路由选择器，支持多种负载均衡策略
// 每十秒会尝试更新服务ip，若有并发获取或服务发现错误，返回旧值
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

type CachedHttpSelector struct {
	lbPolicy    string
	serviceName string
	//多版本路由
	targetCache *cache.SingleCacheEntry[map[string]selector.Selector[string]]

	discoveryType string
}

type CachedHttpSelectorConfig struct {
	LbPolicy            string
	ServiceName         string
	CacheExpireDuration time.Duration
	DiscoveryType       string
}

func NewCachedHttpSelector(config CachedHttpSelectorConfig) *CachedHttpSelector {
	st := &CachedHttpSelector{
		lbPolicy:      config.LbPolicy,
		serviceName:   config.ServiceName,
		discoveryType: config.DiscoveryType,
	}
	cacheExpireDuration := 10 * time.Second
	if config.CacheExpireDuration > 0 {
		cacheExpireDuration = config.CacheExpireDuration
	}
	entry, _ := cache.NewSingleCacheEntry[map[string]selector.Selector[string]](func(ctx context.Context) (map[string]selector.Selector[string], error) {
		//consul拿服务信息
		nodesMap, err := st.serviceMultiVersionNodes(config.ServiceName)
		if err != nil {
			return nil, err
		}
		return st.convert(nodesMap, config.LbPolicy), nil
	}, cacheExpireDuration)
	st.targetCache = entry
	return st
}

func (c *CachedHttpSelector) Select(ctx context.Context, key ...string) (selector.Node[string], error) {
	slrMap, err := c.targetCache.LoadData(ctx)
	if err != nil {
		return selector.Node[string]{}, err
	}
	return c.getFromCache(ctx, slrMap)
}

func (c *CachedHttpSelector) getFromCache(ctx context.Context, slr map[string]selector.Selector[string]) (selector.Node[string], error) {
	ver := rpc.GetHeaders(ctx).Get(rpc.ApiVersion)
	if ver == "" {
		ver = cmd.GetVersion()
	}
	hit, ok := slr[ver]
	if !ok {
		hit = slr[common.DefaultVersion]
	}
	return hit.Select(ctx)
}

func (c *CachedHttpSelector) convert(nodesMap map[string][]selector.Node[string], lbPolicy string) map[string]selector.Selector[string] {
	ret := make(map[string]selector.Selector[string], len(nodesMap))
	slrFn, ok := selector.FindNewSelectorFunc[string](lbPolicy)
	if !ok {
		slrFn = selector.NewRoundRobinSelector[string]
	}
	for ver, nodes := range nodesMap {
		slr, err := slrFn(nodes)
		if err == nil {
			ret[ver] = slr
		}
	}
	return ret
}

func (c *CachedHttpSelector) serviceMultiVersionNodes(serviceName string) (map[string][]selector.Node[string], error) {
	info, err := discovery.GetServiceInfoByDiscoveryType(serviceName, c.discoveryType)
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, errors.New("can not find ip address")
	}
	res := make(map[string][]selector.Node[string])
	//默认版本节点先初始化
	res[common.DefaultVersion] = make([]selector.Node[string], 0)
	i := 0
	for _, item := range info {
		n := selector.Node[string]{
			Id:     strconv.Itoa(i),
			Weight: item.Weight,
			Data:   fmt.Sprintf("%s:%d", item.Addr, item.Port),
		}
		version := common.DefaultVersion
		if item.Version != "" {
			version = item.Version
		}
		ns, ok := res[version]
		if ok {
			res[version] = append(ns, n)
		} else {
			res[version] = append(make([]selector.Node[string], 0), n)
		}
		if version != common.DefaultVersion {
			res[common.DefaultVersion] = append(res[common.DefaultVersion], n)
		}
		i += 1
	}
	return res, nil
}
