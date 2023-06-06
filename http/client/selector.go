package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/discovery"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/selector"
	"strconv"
	"sync"
	"time"
)

// 缓存服务ip的路由选择器，支持多种负载均衡策略
// 每十秒会尝试更新服务ip，若有并发获取或服务发现错误，返回旧值
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

var (
	httpClientCacheDurationSec int
)

func init() {
	// 服务发现过期时间
	sec := property.GetInt("http.client.cacheDurationSec")
	if sec <= 0 {
		sec = 30
	}
	httpClientCacheDurationSec = 30
}

type CachedHttpSelector struct {
	LbPolicy    string
	ServiceName string
	//多版本路由
	cache   map[string]selector.Selector
	cacheMu sync.RWMutex

	expireTime time.Time
}

func (c *CachedHttpSelector) Select(ctx context.Context, key ...string) (node selector.Node, err error) {
	c.cacheMu.RLock()
	oldCache := c.cache
	oldExpireTime := c.expireTime
	c.cacheMu.RUnlock()
	if oldExpireTime.After(time.Now()) {
		logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache still valid")
		node, err = c.getFromCache(ctx, oldCache)
		return
	}
	//首次加载
	if c.expireTime.IsZero() {
		logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache is empty")
		c.cacheMu.Lock()
		defer c.cacheMu.Unlock()
		//双重校验
		if !c.expireTime.IsZero() {
			node, err = c.getFromCache(ctx, c.cache)
			return
		}
		//consul拿服务信息
		nodesMap, err2 := serviceMultiVersionNodes(c.ServiceName)
		if err2 != nil {
			//获取信息失败
			err = err2
			return
		}
		//赋值
		newCache := convert(nodesMap, c.LbPolicy)
		newExpireTime := time.Now().Add(time.Duration(httpClientCacheDurationSec) * time.Second)
		c.cache = newCache
		c.expireTime = newExpireTime
		node, err = c.getFromCache(ctx, newCache)
		logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache get service:", node.Data)
		return
	} else {
		logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache is expired")
		//到期并发冲突
		if c.cacheMu.TryLock() {
			defer c.cacheMu.Unlock()
			nodesMap, err2 := serviceMultiVersionNodes(c.ServiceName)
			logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache read new cache")
			if err2 == nil {
				newCache := convert(nodesMap, c.LbPolicy)
				newExpireTime := time.Now().Add(time.Duration(httpClientCacheDurationSec) * time.Second)
				c.cache = newCache
				c.expireTime = newExpireTime
				node, err = c.getFromCache(ctx, newCache)
				logger.Logger.WithContext(ctx).Debug(c.ServiceName, " http cache get service:", node.Data)
				return
			}
		}
		logger.Logger.Debug(c.ServiceName, " http cache read old cache")
		//抢不到锁或更新失败使用老数据
		node, err = c.getFromCache(ctx, oldCache)
		return
	}
}

func (c *CachedHttpSelector) getFromCache(ctx context.Context, slr map[string]selector.Selector) (node selector.Node, err error) {
	hit, ok := slr[cmd.GetVersion()]
	if !ok {
		node, err = slr[common.DefaultVersion].Select(ctx)
		return
	}
	node, err = hit.Select(ctx)
	return
}

func convert(nodesMap map[string][]selector.Node, lbPolicy string) map[string]selector.Selector {
	cache := make(map[string]selector.Selector, len(nodesMap))
	for ver, nodes := range nodesMap {
		slr, err := selector.NewSelectorFuncMap[lbPolicy](nodes)
		if err == nil {
			cache[ver] = slr
		}
	}
	return cache
}

func serviceMultiVersionNodes(serviceName string) (map[string][]selector.Node, error) {
	info, err := discovery.GetServiceInfo(serviceName)
	if err != nil {
		return nil, err
	}
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
