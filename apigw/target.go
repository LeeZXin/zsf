package apigw

import (
	"github.com/LeeZXin/zsf-utils/listutil"
	"github.com/LeeZXin/zsf-utils/selector"
	"net/http"
)

const (
	DiscoveryTargetType = "discovery"
	DomainTargetType    = "domain"
	MockTargetType      = "mock"
)

func mockTarget(config RouterConfig, _ *http.Client) (hostSelector, rpcExecutor) {
	return &emptySelector{}, &mockExecutor{
		mockContent: config.MockContent,
	}
}

func discoveryTarget(config RouterConfig, httpClient *http.Client) (hostSelector, rpcExecutor) {
	return &ipPortSelector{
			serviceName: config.ServiceName,
		}, &httpExecutor{
			httpClient: httpClient,
		}
}

func domainTarget(config RouterConfig, httpClient *http.Client) (hostSelector, rpcExecutor) {
	targets := config.Targets
	nodes, _ := listutil.Map(targets, func(t Target) (selector.Node[string], error) {
		return selector.Node[string]{
			Data:   t.Target,
			Weight: t.Weight,
		}, nil
	})
	selectorFunc, _ := selector.FindNewSelectorFunc[string](config.TargetLbPolicy)
	return &selectorWrapper{
			Selector: selectorFunc(nodes),
		}, &httpExecutor{
			httpClient: httpClient,
		}
}
