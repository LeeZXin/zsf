package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"zsf/rpc"
	"zsf/selector"
)

// client封装
// 仅支持contentType="app/json;charset=utf-8"请求发送
// 下游是http
// 支持skyWalking的传递

const (
	JSON_CONTENT_TYPE = "application/json;charset=utf-8"
)

var (
	supportedLbPolicy = map[string]selector.LbPolicy{
		"round_robin":          selector.RoundRobinPolicy,
		"weighted_round_robin": selector.WeightedRoundRobinPolicy,
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

func newHttpClient(maxIdleConns int, idleConnTimeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: defaultTransportDialContext(&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}),
			ForceAttemptHTTP2: true,
			MaxIdleConns:      maxIdleConns,
			IdleConnTimeout:   idleConnTimeout,
		},
		Timeout: 30 * time.Second,
	}
}

func defaultTransportDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return dialer.DialContext
}

type Client interface {
	Init()
	Get(ctx context.Context, path string, resp any, opts ...Option) error
	Post(ctx context.Context, path string, req, resp any, opts ...Option) error
	Put(ctx context.Context, path string, req, resp any, opts ...Option) error
	Delete(ctx context.Context, path string, req, resp any, opts ...Option) error
	Close()
}

type ClientImpl struct {
	ServiceName  string
	LbPolicy     selector.LbPolicy
	st           selector.Selector
	http         *http.Client
	Interceptors []ClientInterceptor
}

func (c *ClientImpl) Init() {
	if c.LbPolicy == "" {
		c.LbPolicy = selector.RoundRobinPolicy
	} else {
		_, ok := selector.NewSelectorFuncMap[c.LbPolicy]
		if !ok {
			c.LbPolicy = selector.RoundRobinPolicy
		}
	}
	c.st = &cachedHttpSelector{
		LbPolicy:    c.LbPolicy,
		ServiceName: c.ServiceName,
	}
	_ = c.st.Init()
	c.http = newHttpClient(10, time.Minute)
}

func (c *ClientImpl) Close() {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func (c *ClientImpl) Get(ctx context.Context, path string, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodGet, "", nil, resp, opts...)
}
func (c *ClientImpl) Post(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPost, JSON_CONTENT_TYPE, req, resp, opts...)
}
func (c *ClientImpl) Put(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPut, JSON_CONTENT_TYPE, req, resp, opts...)
}
func (c *ClientImpl) Delete(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodDelete, JSON_CONTENT_TYPE, req, resp, opts...)
}

func (c *ClientImpl) send(ctx context.Context, path, method, contentType string, req, resp any, opts ...Option) error {
	node, err := c.st.Select()
	if err != nil {
		return err
	}
	var reqBytes []byte
	if req != nil {
		reqBytes, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}
	var dopt dialOption
	if opts != nil {
		for _, opt := range opts {
			opt.apply(&dopt)
		}
	}
	host := node.Data.(string)
	url := "http://" + host
	if !strings.HasPrefix(path, "/") {
		url += "/"
	}
	url += path
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBytes))
	if err != nil {
		return err
	}
	header := dopt.header
	if header != nil {
		for k, v := range header {
			request.Header.Set(k, v)
		}
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	invoker := func(request *http.Request) (*http.Response, error) {
		return c.http.Do(request)
	}
	request.Header.Set(rpc.Target, c.ServiceName)
	wrapper := interceptorsWrapper{is: c.Interceptors}
	respBody, err := wrapper.intercept(request, invoker)
	if err != nil {
		return err
	}
	defer func() {
		_ = respBody.Body.Close()
	}()
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
