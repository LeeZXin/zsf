package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/selector"
	"github.com/LeeZXin/zsf/util/httputil"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// client封装
// 仅支持contentType="app/json;charset=utf-8"请求发送
// 下游是http
// 支持skyWalking的传递

const (
	JSON_CONTENT_TYPE = "application/json;charset=utf-8"
)

var (
	supportedLbPolicy = map[string]bool{
		selector.RoundRobinPolicy:         true,
		selector.WeightedRoundRobinPolicy: true,
	}
)

type dialOption struct {
	header map[string]string
}

type Option interface {
	apply(*dialOption)
}

type headerOption struct {
	header map[string]string
}

func (o *headerOption) apply(option *dialOption) {
	option.header = o.header
}

func WithHeader(header map[string]string) Option {
	return &headerOption{
		header: header,
	}
}

func defaultTransportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

type Client interface {
	Select(ctx context.Context) (string, error)
	Get(ctx context.Context, path string, resp any, opts ...Option) error
	Post(ctx context.Context, path string, req, resp any, opts ...Option) error
	Put(ctx context.Context, path string, req, resp any, opts ...Option) error
	Delete(ctx context.Context, path string, req, resp any, opts ...Option) error
	Close()
}

type Impl struct {
	ServiceName   string
	LbPolicy      string
	routeSelector *CachedHttpSelector
	http          *http.Client
	Interceptors  []Interceptor
}

func (c *Impl) Init() {
	if c.LbPolicy == "" {
		c.LbPolicy = selector.RoundRobinPolicy
	} else {
		_, ok := selector.NewSelectorFuncMap[c.LbPolicy]
		if !ok {
			c.LbPolicy = selector.RoundRobinPolicy
		}
	}
	c.routeSelector = NewCachedHttpSelector(c.LbPolicy, c.ServiceName)
	c.http = httputil.NewRetryableHttpClient()
}

func (c *Impl) Close() {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func (c *Impl) Select(ctx context.Context) (string, error) {
	node, err := c.routeSelector.Select(ctx)
	if err != nil {
		return "", err
	}
	return node.Data.(string), nil
}

func (c *Impl) Get(ctx context.Context, path string, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodGet, "", nil, resp, opts...)
}
func (c *Impl) Post(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPost, JSON_CONTENT_TYPE, req, resp, opts...)
}
func (c *Impl) Put(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPut, JSON_CONTENT_TYPE, req, resp, opts...)
}
func (c *Impl) Delete(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodDelete, JSON_CONTENT_TYPE, req, resp, opts...)
}

func (c *Impl) send(ctx context.Context, path, method, contentType string, req, resp any, opts ...Option) error {
	// 获取服务ip
	node, err := c.routeSelector.Select(ctx)
	if err != nil {
		return err
	}
	// request
	var reqBytes []byte
	if req != nil {
		reqBytes, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}
	// 加载选项
	var apply dialOption
	if opts != nil {
		for _, opt := range opts {
			opt.apply(&apply)
		}
	}
	// 拼接host
	host := node.Data.(string)
	url := "http://" + host
	if !strings.HasPrefix(path, "/") {
		url += "/"
	}
	url += path
	// request
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBytes))
	if err != nil {
		return err
	}
	// 塞header
	header := apply.header
	if header != nil {
		for k, v := range header {
			request.Header.Set(k, v)
		}
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	// 塞target信息
	request.Header.Set(rpc.Target, c.ServiceName)

	invoker := func(request *http.Request) (*http.Response, error) {
		return c.http.Do(request)
	}
	// 执行拦截器
	wrapper := interceptorsWrapper{interceptorList: c.Interceptors}
	respBody, err := wrapper.intercept(request, invoker)
	if err != nil {
		return err
	}
	defer respBody.Body.Close()
	if respBody.StatusCode < http.StatusBadRequest {
		respBytes, err := io.ReadAll(respBody.Body)
		if err != nil {
			return err
		}
		if resp != nil {
			return json.Unmarshal(respBytes, resp)
		}
	} else {
		return errors.New("request error with code:" + strconv.Itoa(respBody.StatusCode))
	}
	return nil
}
