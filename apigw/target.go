package apigw

import (
	"github.com/LeeZXin/zsf-utils/listutil"
	"net/http"
)

const (
	DiscoveryTargetType = "discovery"
	DomainTargetType    = "domain"
	MockTargetType      = "mock"
)

func mockTarget(config RouterConfig, _ *http.Client) (hostSelector, rpcExecutor) {
	return new(nilSelector), &mockExecutor{
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
	var hs hostSelector
	switch config.TargetLbPolicy {
	case RoundRobinPolicy:
		targets, _ := listutil.Map(config.Targets, func(t Target) (string, error) {
			return t.Target, nil
		})
		hs = newRoundRobinSelector(targets)
	case WeightedRoundRobinPolicy:
		targets, _ := listutil.Map(config.Targets, func(t Target) (weightedTarget, error) {
			weight := t.Weight
			if weight <= 0 {
				weight = 1
			}
			return weightedTarget{
				weight: weight,
				target: t.Target,
			}, nil
		})
		hs = newWeightedRoundRobinSelector(targets)
	default:
		hs = new(emptyTargetsSelector)
	}
	return hs, &httpExecutor{
		httpClient: httpClient,
	}
}
