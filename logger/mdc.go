/*
本文件实现 TraceId 上下文传递（MDC - Mapped Diagnostic Context）。

设计要点：context 中存储 *string（可变指针）而非 string（不可变值），
TraceId 已存在时原地更新，不创建新 context。

原因：一个请求可能跨越多个中间件和 goroutine，共享同一个 context 树。
原地更新保证链路内所有持有该 context 的地方都能读到最新 TraceId，
同时避免高并发下频繁创建 context 对象。

注意：这偏离了 context 包"不可变"的建议，仅在受控的请求链路内使用。
*/
package logger

import (
	"context"
)

/*
mdcKey 是 context 中存储 TraceId 的 key 类型。

使用空结构体类型而非字符串，避免与其他包的 context key 冲突。
*/
type mdcKey struct{}

/*
AddTraceId 将 TraceId 注入 context。

  - ctx == nil：基于 context.Background() 注入
  - ctx 中已有 TraceId：原地更新值，不创建新 context
  - 否则：context.WithValue 注入

使用场景：HTTP 中间件生成/接收 TraceId、gRPC 拦截器提取 metadata、
异步任务延续 TraceId（配合 CopyTraceId）。
*/
func AddTraceId(ctx context.Context, traceId string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if v := ctx.Value(mdcKey{}); v != nil {
		*v.(*string) = traceId
		return ctx
	}
	t := new(string)
	*t = traceId
	return context.WithValue(ctx, mdcKey{}, t)
}

/*
GetTraceId 从 context 中提取 TraceId，ctx 为 nil 或未注入时返回 "-"。

返回 "-" 而非空串，是为了在日志中明确标识"无 TraceId"的状态，
避免被误认为日志格式异常。
*/
func GetTraceId(ctx context.Context) string {
	if ctx == nil {
		return "-"
	}
	v := ctx.Value(mdcKey{})
	if v == nil {
		return "-"
	}
	return *v.(*string)
}

/*
CopyTraceId 将 ctx 的 TraceId 复制到全新的 context 中。

适用场景：启动异步 goroutine 时延续请求的 TraceId，但不共享原 context 的生命周期
（原 ctx 可能因超时被取消，异步任务需要独立的 context）。

	traceCtx := logger.CopyTraceId(ctx)
	go func() {
	    logger.Ctx(traceCtx).Info().Msg("async task started")
	}()
*/
func CopyTraceId(ctx context.Context) context.Context {
	return AddTraceId(context.Background(), GetTraceId(ctx))
}
