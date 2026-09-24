// Package sentinelutil 封装 Sentinel（sentinel-golang）的熔断与限流规则构造，
// 并提供统一的资源入口包装（Entry）。
// 包初始化时会基于静态配置自动初始化 Sentinel，并使用静默 logger 避免向磁盘输出日志。
package sentinelutil

import (
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"
)

// ErrorCountRule 构造按错误数量触发的熔断规则。
// 参数:
//   - resource: 资源名，需与 Entry 调用时的资源名一致
//   - retryTimeoutMs: 熔断持续时间（毫秒），熔断期间请求快速失败
//   - minRequestAmount: 触发熔断统计所需的最小请求数（静默阈值）
//   - statIntervalMs: 错误统计窗口（毫秒）
//   - errCount: 统计窗口内的错误数阈值，超过即熔断
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

// ErrorRatioRule 构造按错误比例触发的熔断规则，errRatio 为 0~1 的错误率阈值。
// 其余参数（resource、retryTimeoutMs、minRequestAmount、statIntervalMs）语义与 ErrorCountRule 一致。
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

// SlowRatioRule 构造按慢调用比例触发的熔断规则。
// 参数:
//   - resource、retryTimeoutMs、minRequestAmount、statIntervalMs: 语义与 ErrorCountRule 一致
//   - maxAllowedRtMs: 请求耗时超过该毫秒数即记为慢调用
//   - slowRatio: 慢调用比例阈值（0~1），超过即熔断
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

// QueueingRule 构造匀速排队（Throttling）限流规则：请求按阈值均摊的间隔放行，超出排队上限直接拒绝。
// 参数:
//   - resource: 资源名，需与 Entry 调用时的资源名一致
//   - threshold: 每秒允许通过的请求数（QPS）
//   - maxQueueingTimeMs: 排队等待的最长时间（毫秒），超时请求被拒绝
func QueueingRule(resource string, threshold float64, maxQueueingTimeMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Throttling,   // 流控效果为匀速排队
		Threshold:              threshold,         // 请求的间隔控制在 1000/10=100 ms
		MaxQueueingTimeMs:      maxQueueingTimeMs, // 最长排队等待时间
	}
}

// SlidingWindowRule 构造固定窗口限流规则：窗口内请求数超过阈值直接拒绝。
// 参数:
//   - resource: 资源名，需与 Entry 调用时的资源名一致
//   - threshold: 窗口内允许的最大请求数
//   - statIntervalInMs: 统计窗口长度（毫秒）
func SlidingWindowRule(resource string, threshold float64, statIntervalInMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.Direct,
		ControlBehavior:        flow.Reject,
		Threshold:              threshold,
		StatIntervalInMs:       statIntervalInMs,
	}
}

// QpsRule 构造每秒 QPS 限流规则（1 秒固定窗口，超出阈值直接拒绝），等价于
// SlidingWindowRule(resource, threshold, 1000)。
func QpsRule(resource string, threshold float64) *flow.Rule {
	return SlidingWindowRule(resource, threshold, 1000)
}

// WarmUpRule 构造慢启动限流规则：冷启动阶段逐步放量至满阈值，避免服务刚启动时被瞬间打满。
// 参数:
//   - resource: 资源名，需与 Entry 调用时的资源名一致
//   - warmUpPeriodSec: 预热时长（秒），预热因子固定为 3
//   - threshold: 预热结束后的满速 QPS 阈值
//   - statIntervalInMs: 统计窗口长度（毫秒）
func WarmUpRule(resource string, warmUpPeriodSec uint32, threshold float64, statIntervalInMs uint32) *flow.Rule {
	return &flow.Rule{
		Resource:               resource,
		TokenCalculateStrategy: flow.WarmUp,
		ControlBehavior:        flow.Reject,
		WarmUpColdFactor:       3,
		WarmUpPeriodSec:        warmUpPeriodSec,
		Threshold:              threshold,
		StatIntervalInMs:       statIntervalInMs,
	}
}
