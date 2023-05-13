package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/selector"
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
	Get(ctx context.Context, path string, resp any, opts ...Option) error
	Post(ctx context.Context, path string, req, resp any, opts ...Option) error
	Put(ctx context.Context, path string, req, resp any, opts ...Option) error
	Delete(ctx context.Context, path string, req, resp any, opts ...Option) error
	Close()
}

type Impl struct {
	ServiceName  string
	LbPolicy     string
	st           selector.Selector
	http         *http.Client
	Interceptors []Interceptor
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
	c.st = &cachedHttpSelector{
		lbPolicy:    c.LbPolicy,
		serviceName: c.ServiceName,
	}
	c.http = newRetryableHttpClient()
}

func (c *Impl) Close() {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
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
