package header

import (
	"context"
)

// rpc通用header信息
// 用于http和grpc之间header的传递

type headerKey struct{}

type Header map[string]string

func (h Header) Get(key string) string {
	return h[key]
}

func (h Header) Set(key, value string) {
	h[key] = value
}

const (
	TraceId    = "z-trace-id"
	Shadow     = "z-shadow"
	ApiVersion = "z-api-version"
	Prefix     = "z-"
	Source     = "z-source"
	Target     = "z-target"

	// PrefixForSw 为了skywalking传递header用的
	PrefixForSw = "z-sw-"
)

func GetHeaders(ctx context.Context) Header {
	if ctx != nil {
		value := ctx.Value(headerKey{})
		if value != nil {
			return value.(Header)
		}
	}
	return make(Header)
}

func SetHeaders(ctx context.Context, content map[string]string) context.Context {
	if ctx != nil {
		return context.WithValue(ctx, headerKey{}, Header(content))
	}
	return context.Background()
}

func AddHeaders(ctx context.Context, content map[string]string) context.Context {
	if ctx != nil {
		headers := GetHeaders(ctx)
		for k, v := range content {
			headers.Set(k, v)
		}
		return SetHeaders(ctx, headers)
	}
	return context.Background()
}

func IsShadow(ctx context.Context) bool {
	return GetHeaders(ctx).Get(Shadow) != ""
}
