package sentinelutil

import (
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
)

// Entry 在 Sentinel 保护下执行 fn：先按 resource 进入（Inbound Web 流量），
// 被限流/熔断时直接返回 Sentinel 的错误且不执行 fn；否则执行 fn 并返回其结果。
// 注意：fn 内对同一资源嵌套调用 Entry 会使该资源的调用计数翻倍，应避免。
func Entry[R any](resource string, fn func() (R, error)) (R, error) {
	entry, berr := sentinel.Entry(resource,
		sentinel.WithResourceType(base.ResTypeWeb),
		sentinel.WithTrafficType(base.Inbound))
	if berr != nil {
		var r R
		return r, berr
	}
	defer entry.Exit()
	return fn()
}
