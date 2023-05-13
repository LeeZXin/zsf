package logger

import "context"

//日志关键信息打印
//从context获取

type MDCKey struct{}

type MDC map[string]string

func (h MDC) Get(key string) string {
	return h[key]
}

const (
	TraceId = "z-trace-id"
)

func GetMDC(ctx context.Context) MDC {
	value := ctx.Value(MDCKey{})
	if value != nil {
		return value.(MDC)
	}
	return make(MDC)
}

func AppendToMDC(ctx context.Context, content map[string]string) context.Context {
	if ctx != nil {
		return context.WithValue(ctx, MDCKey{}, MDC(content))
	}
	return context.Background()
}
