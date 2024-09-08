package apigw

import (
	"github.com/LeeZXin/zsf-utils/listutil"
	"net/http"
)

type TargetType string

const (
	DiscoveryTargetType TargetType = "discovery"
	DomainTargetType    TargetType = "domain"
	MockTargetType      TargetType = "mock"
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
		targets := listutil.MapNe(config.Targets, func(t Target) string {
			return t.Target
		})
		hs = newRoundRobinSelector(targets)
	case WeightedRoundRobinPolicy:
		targets := listutil.MapNe(config.Targets, func(t Target) weightedTarget {
			weight := t.Weight
			if weight <= 0 {
				weight = 1
			}
			return weightedTarget{
				weight: weight,
				target: t.Target,
			}
		})
		hs = newWeightedRoundRobinSelector(targets)
	default:
		hs = new(emptyTargetsSelector)
	}
	return hs, &httpExecutor{
		httpClient: httpClient,
	}
}
