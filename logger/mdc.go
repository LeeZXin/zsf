package logger

import (
	"context"
	"github.com/LeeZXin/zsf-utils/idutil"
)

//日志关键信息打印
//从context获取

type MDCKey struct{}

type MDC map[string]string

func (h MDC) Get(key string) string {
	return h[key]
}

func (h MDC) Set(key, val string) {
	h[key] = val
}

const (
	TraceId = "z-trace-id"
)

func GetMDC(ctx context.Context) MDC {
	if ctx != nil {
		value := ctx.Value(MDCKey{})
		if value != nil {
			return value.(MDC)
		}
	}
	return make(MDC)
}

func AppendToMDC(ctx context.Context, content map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	mdc := GetMDC(ctx)
	n := make(MDC, len(mdc)+len(content))
	for k, v := range mdc {
		n.Set(k, v)
	}
	for k, v := range content {
		n.Set(k, v)
	}
	return context.WithValue(ctx, MDCKey{}, n)
}

func GenTraceId(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	uuid := idutil.RandomUuid()
	return AppendToMDC(ctx, map[string]string{
		TraceId: uuid,
	}), uuid
}

func CopyMDC(ctx context.Context) context.Context {
	return AppendToMDC(context.Background(), GetMDC(ctx))
}

func GetTraceId(ctx context.Context) string {
	return GetMDC(ctx).Get(TraceId)
}
