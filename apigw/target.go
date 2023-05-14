package apigw

import (
	"errors"
	"github.com/LeeZXin/zsf/http/client"
	"github.com/LeeZXin/zsf/selector"
	"strconv"
)

const (
	DiscoveryTargetType = "discovery"
	DomainTargetType    = "domain"
	MockTargetType      = "mock"
)

var (
	newTargetFuncMap = map[string]NewTargetFunc{
		MockTargetType: func(config RouterConfig) (selector.Selector, RpcExecutor, error) {
			if config.MockContent == nil {
				return nil, nil, errors.New("nil mock content")
			}
			return nil, &MockExecutor{MockContent: config.MockContent}, nil
		},
		DiscoveryTargetType: func(config RouterConfig) (selector.Selector, RpcExecutor, error) {
			serviceName := config.ServiceName
			if serviceName == "" {
				return nil, nil, errors.New("empty serviceName")
			}
			st := &client.CachedHttpSelector{
				LbPolicy:    config.TargetLbPolicy,
				ServiceName: serviceName,
			}
			return st, &HttpExecutor{}, nil
		},
		DomainTargetType: func(config RouterConfig) (selector.Selector, RpcExecutor, error) {
			targets := config.Targets
			if len(targets) == 0 {
				return nil, nil, errors.New("empty targets")
			}
			nodes := make([]selector.Node, len(targets))
			for i, target := range targets {
				nodes[i] = selector.Node{
					Id:     strconv.Itoa(i),
					Data:   target.Target,
					Weight: target.Weight,
				}
			}
			selectorFunc, ok := selector.NewSelectorFuncMap[config.TargetLbPolicy]
			if !ok {
				return nil, nil, errors.New("wrong lb policy")
			}
			st, err := selectorFunc(nodes)
			if err != nil {
				return nil, nil, err
			}
			return st, &HttpExecutor{}, nil
		},
	}
)

type NewTargetFunc func(config RouterConfig) (selector.Selector, RpcExecutor, error)

type NewSelectorFunc func([]selector.Node) (selector.Selector, error)
