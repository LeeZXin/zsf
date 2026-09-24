package sentinelutil

import (
	"testing"

	"github.com/alibaba/sentinel-golang/core/flow"
)

func TestWarmUpRuleUsesWarmUpStrategy(t *testing.T) {
	r := WarmUpRule("res", 10, 100, 1000)
	if r.TokenCalculateStrategy != flow.WarmUp {
		t.Fatalf("WarmUpRule 应为 WarmUp 策略，got %v", r.TokenCalculateStrategy)
	}
	if r.WarmUpPeriodSec != 10 || r.Threshold != 100 {
		t.Fatalf("预热参数丢失: %+v", r)
	}
}
