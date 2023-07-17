package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/cache"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/logger"
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
	targetCache *cache.SingleCacheEntry

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
	entry, _ := cache.NewSingleCacheEntry(func(ctx context.Context) (any, error) {
		//consul拿服务信息
		nodesMap, err := st.serviceMultiVersionNodes(config.ServiceName, ctx)
		if err != nil {
			return nil, err
		}
		return st.convert(nodesMap, config.LbPolicy), nil
	}, cacheExpireDuration)
	st.targetCache = entry
	return st
}

func (c *CachedHttpSelector) Select(ctx context.Context, key ...string) (selector.Node, error) {
	slrMap, err := c.targetCache.LoadData(ctx)
	if err != nil {
		return selector.Node{}, err
	}
	return c.getFromCache(ctx, slrMap.(map[string]selector.Selector))
}

func (c *CachedHttpSelector) getFromCache(ctx context.Context, slr map[string]selector.Selector) (selector.Node, error) {
	hit, ok := slr[cmd.GetVersion()]
	if !ok {
		hit = slr[common.DefaultVersion]
	}
	return hit.Select(ctx)
}

func (c *CachedHttpSelector) convert(nodesMap map[string][]selector.Node, lbPolicy string) map[string]selector.Selector {
	ret := make(map[string]selector.Selector, len(nodesMap))
	slrFn, ok := selector.NewSelectorFuncMap[lbPolicy]
	if !ok {
		slrFn = selector.NewSelectorFuncMap[selector.RoundRobinPolicy]
	}
	for ver, nodes := range nodesMap {
		slr, err := slrFn(nodes)
		if err == nil {
			ret[ver] = slr
		}
	}
	return ret
}

func (c *CachedHttpSelector) serviceMultiVersionNodes(serviceName string, ctx context.Context) (map[string][]selector.Node, error) {
	info, err := discovery.GetServiceInfoByDiscoveryType(serviceName, c.discoveryType)
	if err != nil {
		return nil, err
	}
	logger.Logger.WithContext(ctx).Info("load service:", serviceName, " from discovery result:", info)
	if len(info) == 0 {
		return nil, errors.New("can not find ip address")
	}
	res := make(map[string][]selector.Node)
	//默认版本节点先初始化
	res[common.DefaultVersion] = make([]selector.Node, 0)
	i := 0
	for _, item := range info {
		n := selector.Node{
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
			res[version] = append(make([]selector.Node, 0), n)
		}
		if version != common.DefaultVersion {
			res[common.DefaultVersion] = append(res[common.DefaultVersion], n)
		}
		i += 1
	}
	return res, nil
}
