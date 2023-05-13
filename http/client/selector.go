package client

import (
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/selector"
	"sync"
	"time"
)

// 缓存服务ip的路由选择器，支持多种负载均衡策略
// 每十秒会尝试更新服务ip，若有并发获取或服务发现错误，返回旧值
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

type cachedHttpSelector struct {
	lbPolicy    string
	serviceName string

	cache      map[string]selector.Selector
	expireTime time.Time
	cacheMu    sync.RWMutex
}

func (c *cachedHttpSelector) Select(key ...string) (node selector.Node, err error) {
	c.cacheMu.RLock()
	oldCache := c.cache
	oldExpireTime := c.expireTime
	c.cacheMu.RUnlock()
	if oldExpireTime.After(time.Now()) {
		node, err = c.getFromCache(oldCache)
		return
	}
	//首次加载
	if c.expireTime.IsZero() {
		c.cacheMu.Lock()
		//双重校验
		if !c.expireTime.IsZero() {
			c.cacheMu.Unlock()
			node, err = c.getFromCache(c.cache)
			return
		}
		nodesMap, err2 := selector.ServiceMultiVersionNodes(c.serviceName)
		if err2 != nil {
			c.cacheMu.Unlock()
			err = err2
			return
		}
		newCache := convert(nodesMap, c.lbPolicy)
		newExpireTime := time.Now().Add(10 * time.Second)
		c.cache = newCache
		c.expireTime = newExpireTime
		c.cacheMu.Unlock()
		node, err = c.getFromCache(newCache)
		return
	} else {
		if c.cacheMu.TryLock() {
			nodesMap, err2 := selector.ServiceMultiVersionNodes(c.serviceName)
			//如果出错 使用老数据
			if err2 == nil {
				newCache := convert(nodesMap, c.lbPolicy)
				newExpireTime := time.Now().Add(10 * time.Second)
				c.cache = newCache
				c.expireTime = newExpireTime
				c.cacheMu.Unlock()
				node, err = c.getFromCache(newCache)
				return
			}
			c.cacheMu.Unlock()
		}
		node, err = c.getFromCache(oldCache)
		return
	}
}

func (c *cachedHttpSelector) getFromCache(slr map[string]selector.Selector) (node selector.Node, err error) {
	hit, ok := slr[appinfo.Version]
	if !ok {
		node, err = slr[appinfo.DefaultVersion].Select()
		return
	}
	node, err = hit.Select()
	return
}

func convert(nodesMap map[string][]selector.Node, lbPolicy string) (cache map[string]selector.Selector) {
	for ver, nodes := range nodesMap {
		slr, err := selector.NewSelectorFuncMap[lbPolicy](nodes)
		if err == nil {
			cache[ver] = slr
		}
	}
	return
}
