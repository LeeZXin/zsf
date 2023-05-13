package httpclient

import (
	"errors"
	"fmt"
	"strconv"
	"time"
	"zsf/cache"
	"zsf/common"
	"zsf/discovery"
	"zsf/selector"
)

// 缓存服务ip的路由选择器，支持多种负载均衡策略
// 每十秒会尝试更新服务ip，若有并发获取或服务发现错误，返回旧值
// 根据版本号路由，优先发送到相同版本服务，若不存在，发送到其他版本服务

type cachedHttpSelector struct {
	LbPolicy    selector.LbPolicy
	ServiceName string
	cache       *cache.SingleItemCache
}

func (c *cachedHttpSelector) Init() error {
	c.cache = &cache.SingleItemCache{
		ExpireDuration: 10 * time.Second,
		Supplier: func() (any, error) {
			addresses, err := discovery.GetServiceInfo(c.ServiceName)
			if err != nil {
				return nil, err
			}
			if len(addresses) == 0 {
				return nil, errors.New("can not find ip address")
			}
			mn := make(map[string][]*selector.Node)
			//默认版本节点先初始化
			mn[common.DefaultVersion] = make([]*selector.Node, 0)
			i := 0
			for _, item := range addresses {
				n := &selector.Node{
					Id:     strconv.Itoa(i),
					Weight: item.Weight,
					Data:   fmt.Sprintf("%s:%d", item.Address, item.Port),
				}
				version := common.DefaultVersion
				if item.Version != "" {
					version = item.Version
				}
				ns, ok := mn[version]
				if ok {
					mn[version] = append(ns, n)
				} else {
					mn[version] = append(make([]*selector.Node, 0), n)
				}
				if version != common.DefaultVersion {
					mn[common.DefaultVersion] = append(mn[common.DefaultVersion], n)
				}
				i += 1
			}
			ms := make(map[string]selector.Selector, len(mn))
			for ver, ns := range mn {
				st := selector.NewSelectorFuncMap[c.LbPolicy](ns)
				err = st.Init()
				if err != nil {
					return nil, err
				}
				ms[ver] = st
			}
			return ms, nil
		},
	}
	return nil
}

func (c *cachedHttpSelector) Select(key ...string) (*selector.Node, error) {
	data, err := c.cache.Get()
	if data == nil && err != nil {
		return nil, err
	} else {
		m := data.(map[string]selector.Selector)
		st, ok := m[common.Version]
		if ok {
			return st.Select()
		} else {
			return m[common.DefaultVersion].Select()
		}
	}
}
