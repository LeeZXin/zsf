package apigw

import (
	"errors"
	"github.com/LeeZXin/zsf/http/client"
	"github.com/LeeZXin/zsf/selector"
	"net/http"
	"strconv"
	"time"
)

const (
	DiscoveryTargetType = "discovery"
	DomainTargetType    = "domain"
	MockTargetType      = "mock"
)

var (
	newTargetFuncMap = map[string]func(config RouterConfig, httpClient *http.Client) (selector.Selector[string], RpcExecutor, error){
		MockTargetType: func(config RouterConfig, httpClient *http.Client) (selector.Selector[string], RpcExecutor, error) {
			return nil, &mockExecutor{
				mockContent: config.MockContent,
			}, nil
		},
		DiscoveryTargetType: func(config RouterConfig, httpClient *http.Client) (selector.Selector[string], RpcExecutor, error) {
			serviceName := config.ServiceName
			if serviceName == "" {
				return nil, nil, errors.New("empty serviceName")
			}
			return client.NewCachedHttpSelector(client.CachedHttpSelectorConfig{
					LbPolicy:            config.TargetLbPolicy,
					ServiceName:         serviceName,
					CacheExpireDuration: 10 * time.Second,
				}), &httpExecutor{
					httpClient: httpClient,
				}, nil
		},
		DomainTargetType: func(config RouterConfig, httpClient *http.Client) (selector.Selector[string], RpcExecutor, error) {
			targets := config.Targets
			if len(targets) == 0 {
				return nil, nil, errors.New("empty targets")
			}
			nodes := make([]selector.Node[string], len(targets))
			for i, target := range targets {
				nodes[i] = selector.Node[string]{
					Id:     strconv.Itoa(i),
					Data:   target.Target,
					Weight: target.Weight,
				}
			}
			selectorFunc, ok := selector.FindNewSelectorFunc[string](config.TargetLbPolicy)
			if !ok {
				return nil, nil, errors.New("wrong lb policy")
			}
			st, err := selectorFunc(nodes)
			if err != nil {
				return nil, nil, err
			}
			return st, &httpExecutor{
				httpClient: httpClient,
			}, nil
		},
	}
)
