package client

import (
	"context"
	"net/http"
	"time"
)

// headerKey 自定义 HTTP header 的 context key。
type headerKey struct{}

// customHTTPTransport 在请求发出前把 context 中的自定义 header 注入请求。
//
// 官方 go-sdk 目前不提供任意自定义 header 的字段（仅有 OAuth 流程支持），
// 官方代码注释明确预期通过 ctx values + 自定义 RoundTripper 实现认证 header
// （见 streamable.go 中 xcontext.Detach 保留 context values 的说明）。
type customHTTPTransport struct {
	rt http.RoundTripper
}

func (t *customHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	value := req.Context().Value(headerKey{})
	if value != nil {
		header := value.(map[string]string)
		for k, v := range header {
			req.Header.Set(k, v)
		}
	}
	return t.rt.RoundTrip(req)
}

// SetCustomHTTPHeader 把自定义 header 挂到 context 上，
// 通过该 context 创建的 MCP 会话的所有请求都会携带这些 header。
func SetCustomHTTPHeader(ctx context.Context, header map[string]string) context.Context {
	if header == nil {
		return ctx
	}
	return context.WithValue(ctx, headerKey{}, header)
}

// NewCustomHTTPClient 创建携带自定义 header 能力的 http.Client。
func NewCustomHTTPClient() *http.Client {
	// Timeout 仅限制到响应头返回为止（连接、TLS、写请求、读响应头），
	// 避免 endpoint 不可达时请求长时间挂起；
	// Streamable HTTP 长连接建立后流式读取不受影响。
	return &http.Client{
		Transport: &customHTTPTransport{http.DefaultTransport},
		Timeout:   30 * time.Second,
	}
}
