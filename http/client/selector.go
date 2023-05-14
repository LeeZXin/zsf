package client

import (
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/selector"
	"sync"
	"time"
)

// 缓存服务ip的路由选择器，支持多种负载均衡策略
// 每十秒会尝试更新服务ip，若有并发获取或服务发现错误，返回旧值
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

type CachedHttpSelector struct {
	LbPolicy    string
	ServiceName string
	//多版本路由
	cache   map[string]selector.Selector
	cacheMu sync.RWMutex

	expireTime time.Time
}

func (c *CachedHttpSelector) Select(key ...string) (node selector.Node, err error) {
	c.cacheMu.RLock()
	oldCache := c.cache
	oldExpireTime := c.expireTime
	c.cacheMu.RUnlock()
	if oldExpireTime.After(time.Now()) {
		logger.Logger.Debug(c.ServiceName, " http cache still valid")
		node, err = c.getFromCache(oldCache)
		return
	}
	//首次加载
	if c.expireTime.IsZero() {
		logger.Logger.Debug(c.ServiceName, " http cache is empty")
		c.cacheMu.Lock()
		//双重校验
		if !c.expireTime.IsZero() {
			c.cacheMu.Unlock()
			node, err = c.getFromCache(c.cache)
			return
		}
		//consul拿服务信息
		nodesMap, err2 := selector.ServiceMultiVersionNodes(c.ServiceName)
		if err2 != nil {
			//获取信息失败
			c.cacheMu.Unlock()
			err = err2
			return
		}
		//赋值
		newCache := convert(nodesMap, c.LbPolicy)
		newExpireTime := time.Now().Add(10 * time.Second)
		c.cache = newCache
		c.expireTime = newExpireTime
		c.cacheMu.Unlock()
		node, err = c.getFromCache(newCache)
		logger.Logger.Debug(c.ServiceName, " http cache get service:", node.Data)
		return
	} else {
		logger.Logger.Debug(c.ServiceName, " http cache is expired")
		//到期并发冲突
		if c.cacheMu.TryLock() {
			nodesMap, err2 := selector.ServiceMultiVersionNodes(c.ServiceName)
			logger.Logger.Debug(c.ServiceName, " http cache read new cache")
			if err2 == nil {
				newCache := convert(nodesMap, c.LbPolicy)
				newExpireTime := time.Now().Add(10 * time.Second)
				c.cache = newCache
				c.expireTime = newExpireTime
				c.cacheMu.Unlock()
				node, err = c.getFromCache(newCache)
				logger.Logger.Debug(c.ServiceName, " http cache get service:", node.Data)
				return
			}
			c.cacheMu.Unlock()
		}
		logger.Logger.Debug(c.ServiceName, " http cache read old cache")
		//抢不到锁或更新失败使用老数据
		node, err = c.getFromCache(oldCache)
		return
	}
}

func (c *CachedHttpSelector) getFromCache(slr map[string]selector.Selector) (node selector.Node, err error) {
	hit, ok := slr[cmd.GetVersion()]
	if !ok {
		node, err = slr[appinfo.DefaultVersion].Select()
		return
	}
	node, err = hit.Select()
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
