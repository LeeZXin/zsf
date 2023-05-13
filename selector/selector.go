package selector

import (
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/discovery"
	"strconv"
)

//负载均衡策略选择器通用封装
//用于rpc的节点负载均衡或其他负载均衡实现

// lbPolicy 负载均衡策略
// 目前只实现轮询、加权平滑轮询、哈希

const (
	RoundRobinPolicy         = "round_robin"
	WeightedRoundRobinPolicy = "weighted_round_robin"
	HashPolicy               = "hash_policy"
)

var (
	NewSelectorFuncMap = map[string]func([]Node) (Selector, error){
		WeightedRoundRobinPolicy: NewWeightedRoundRobinSelector,
		RoundRobinPolicy:         NewRoundRobinSelector,
		HashPolicy:               NewHashSelector,
	}

	EmptyNodesErr = errors.New("empty nodes")
)

// Selector 路由选择器interface
type Selector interface {
	// Select 选择
	Select(key ...string) (Node, error)
}

// Node 路由节点信息
type Node struct {
	Id     string `json:"id"`
	Data   any    `json:"data"`
	Weight int    `json:"weight"`
}

func ServiceMultiVersionNodes(serviceName string) (map[string][]Node, error) {
	info, err := discovery.GetServiceInfo(serviceName)
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, errors.New("can not find ip address")
	}
	res := make(map[string][]Node)
	//默认版本节点先初始化
	res[appinfo.DefaultVersion] = make([]Node, 0)
	i := 0
	for _, item := range info {
		n := Node{
			Id:     strconv.Itoa(i),
			Weight: item.Weight,
			Data:   fmt.Sprintf("%s:%d", item.Addr, item.Port),
		}
		version := appinfo.DefaultVersion
		if item.Version != "" {
			version = item.Version
		}
		ns, ok := res[version]
		if ok {
			res[version] = append(ns, n)
		} else {
			res[version] = append(make([]Node, 0), n)
		}
		if version != appinfo.DefaultVersion {
			res[appinfo.DefaultVersion] = append(res[appinfo.DefaultVersion], n)
		}
		i += 1
	}
	return res, nil
}
