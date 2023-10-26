package apigw

import (
	"github.com/LeeZXin/zsf-utils/selector"
	"net/http"
	"strconv"
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
	return &httpSelector{
			serviceName: config.ServiceName,
		}, &httpExecutor{
			httpClient: httpClient,
		}
}

func domainTarget(config RouterConfig, httpClient *http.Client) (hostSelector, rpcExecutor) {
	targets := config.Targets
	nodes := make([]selector.Node[string], len(targets))
	for i, target := range targets {
		nodes[i] = selector.Node[string]{
			Id:     strconv.Itoa(i),
			Data:   target.Target,
			Weight: target.Weight,
		}
	}
	selectorFunc, _ := selector.FindNewSelectorFunc[string](config.TargetLbPolicy)
	return &selectorWrapper{
			Selector: selectorFunc(nodes),
		}, &httpExecutor{
			httpClient: httpClient,
		}
}
