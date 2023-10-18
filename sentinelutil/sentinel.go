package sentinelutil

import (
	"github.com/LeeZXin/zsf/logger"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"
)

//sentinel 的常用配置

func init() {
	err := sentinel.InitDefault()
	if err != nil {
		logger.Logger.Panic(err)
	}
}

// ErrorCountRule 错误数量触发熔断
func ErrorCountRule(resource string, retryTimeoutMs uint32, minRequestAmount uint64, statIntervalMs uint32, errCount float64) *circuitbreaker.Rule {
	return &circuitbreaker.Rule{
		Resource:         resource,
		Strategy:         circuitbreaker.ErrorCount,
		RetryTimeoutMs:   retryTimeoutMs,   //熔断后n毫秒内快速失败
		MinRequestAmount: minRequestAmount, //静默请求数量
		StatIntervalMs:   statIntervalMs,   //统计时间周期
		Threshold:        errCount,         //错误数量
	}
}

// ErrorRatioRule 错误比例
func ErrorRatioRule(resource string, retryTimeoutMs uint32, minRequestAmount uint64, statIntervalMs uint32, errRatio float64) *circuitbreaker.Rule {
	return &circuitbreaker.Rule{
		Resource:         resource,
		Strategy:         circuitbreaker.ErrorRatio,
		RetryTimeoutMs:   retryTimeoutMs,
		MinRequestAmount: minRequestAmount,
		StatIntervalMs:   statIntervalMs,
		Threshold:        errRatio,
	}
}

// SlowRatioRule 慢调用比例
func SlowRatioRule(resource string, retryTimeoutMs uint32, minRequestAmount uint64, statIntervalMs uint32, maxAllowedRtMs uint64, slowRatio float64) *circuitbreaker.Rule {
	return &circuitbreaker.Rule{
		Resource:         resource,
		Strategy:         circuitbreaker.SlowRequestRatio,
		RetryTimeoutMs:   retryTimeoutMs,
		MinRequestAmount: minRequestAmount,
		StatIntervalMs:   statIntervalMs,
		MaxAllowedRtMs:   maxAllowedRtMs,
		Threshold:        slowRatio,
	}
}

// QueueingRule 排队策略
func QueueingRule(resource string, threshold float64, maxQueueingTimeMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Throttling,   // 流控效果为匀速排队
		Threshold:              threshold,         // 请求的间隔控制在 1000/10=100 ms
		MaxQueueingTimeMs:      maxQueueingTimeMs, // 最长排队等待时间
	}
}

// SlidingWindowRule 滑动窗口
func SlidingWindowRule(resource string, threshold float64, statIntervalInMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Reject,
		Threshold:              threshold,
		StatIntervalInMs:       statIntervalInMs,
	}
}

// QpsRule 配置
func QpsRule(resource string, threshold float64) *flow.Rule {
	return SlidingWindowRule(resource, threshold, 1000)
}

// WarmUpRule 慢启动
func WarmUpRule(resource string, warmUpPeriodSec uint32, threshold float64, statIntervalInMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Reject,
		WarmUpColdFactor:       3,
		WarmUpPeriodSec:        warmUpPeriodSec,
		Threshold:              threshold,
		StatIntervalInMs:       statIntervalInMs,
	}
}
