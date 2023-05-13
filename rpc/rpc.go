package rpc

import (
	"context"
	"strings"
)

// rpc通用header信息
// 用于http和grpc之间header的传递

type HeaderKey struct{}

type Header map[string]string

func (h Header) Get(key string) string {
	return h[key]
}

const (
	TraceId     = "z-trace-id"
	Prefix      = "z-"
	Source      = "z-source"
	Target      = "z-target"
	PrefixForSw = "z-sw-" //为了skywalking传递header用的
)

func GetHeaders(ctx context.Context) Header {
	value := ctx.Value(HeaderKey{})
	if value != nil {
		return value.(Header)
	}
	return make(Header)
}

func AppendToHeader(ctx context.Context, content map[string]string) context.Context {
	if ctx != nil {
		return context.WithValue(ctx, HeaderKey{}, Header(content))
	}
	return context.Background()
}

func IsShadow(ctx context.Context) bool {
	return strings.HasSuffix(GetHeaders(ctx).Get(TraceId), "-Shadow")
}
