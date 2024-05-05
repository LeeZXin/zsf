package rpcheader

import (
	"context"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf/logger"
)

const (
	TraceId    = "z-trace-id"
	ApiVersion = "z-api-version"
	Prefix     = "z-"
	Source     = "z-source"
	Target     = "z-target"

	// PrefixForSw 为了skywalking传递header用的
	PrefixForSw = "z-sw-"
)

type headerKey struct{}

type Header map[string]string

func (h Header) Get(key string) string {
	return h[key]
}

func (h Header) Set(key, val string) {
	h[key] = val
}

func GetHeaders(ctx context.Context) Header {
	value := ctx.Value(headerKey{})
	if value != nil {
		return value.(Header)
	}
	return make(Header)
}

func AppendToHeaders(ctx context.Context, headers map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	h := GetHeaders(ctx)
	ret := make(Header, len(headers)+len(h))
	for k, v := range h {
		ret.Set(k, v)
	}
	for k, v := range headers {
		ret.Set(k, v)
	}
	return context.WithValue(ctx, headerKey{}, ret)
}

func SetHeaders(ctx context.Context, headers map[string]string) context.Context {
	return AppendToHeaders(ctx, headers)
}

func GenTraceId(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	uuid := idutil.RandomUuid()
	return AppendToHeaders(ctx, map[string]string{
		TraceId: uuid,
	}), uuid
}

func GenTraceIdIfAbsent(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	headers := GetHeaders(ctx)
	uuid := headers.Get(TraceId)
	if uuid != "" {
		return ctx, uuid
	}
	uuid = idutil.RandomUuid()
	return AppendToHeaders(ctx, map[string]string{
		TraceId: uuid,
	}), uuid
}

func NewCtxFromOldCtx(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	headers := GetHeaders(ctx)
	uuid := headers.Get(TraceId)
	if uuid == "" {
		uuid = idutil.RandomUuid()
	}
	ctx = logger.AppendToMDC(context.Background(), map[string]string{
		logger.TraceId: uuid,
	})
	ctx = AppendToHeaders(ctx, map[string]string{
		TraceId: uuid,
	})
	return ctx, uuid
}
