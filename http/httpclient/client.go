package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf/rpcheader"
	"github.com/LeeZXin/zsf/services/discovery"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// client封装
// 仅支持contentType="app/json;charset=utf-8"请求发送
// 下游是http
// 支持skyWalking的传递

const (
	JsonContentType = "application/json;charset=utf-8"
)

type dialOption struct {
	header map[string]string
}

type Option func(*dialOption)

type headerOption struct {
	header map[string]string
}

func WithHeader(header map[string]string) Option {
	return func(o *dialOption) {
		o.header = header
	}
}

type Client interface {
	Get(ctx context.Context, path string, resp any, opts ...Option) error
	Post(ctx context.Context, path string, req, resp any, opts ...Option) error
	Put(ctx context.Context, path string, req, resp any, opts ...Option) error
	Delete(ctx context.Context, path string, req, resp any, opts ...Option) error
	Close()
}

type clientImpl struct {
	ServiceName  string
	http         *http.Client
	Interceptors []Interceptor
}

func (c *clientImpl) init() {
	c.http = httputil.NewRetryableHttpClient()
}

func (c *clientImpl) Close() {
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func (c *clientImpl) Get(ctx context.Context, path string, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodGet, "", nil, resp, opts...)
}
func (c *clientImpl) Post(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPost, JsonContentType, req, resp, opts...)
}
func (c *clientImpl) Put(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodPut, JsonContentType, req, resp, opts...)
}
func (c *clientImpl) Delete(ctx context.Context, path string, req, resp any, opts ...Option) error {
	return c.send(ctx, path, http.MethodDelete, JsonContentType, req, resp, opts...)
}

func (c *clientImpl) send(ctx context.Context, path, method, contentType string, req, resp any, opts ...Option) error {
	// 获取服务ip
	host, err := discovery.PickOneHost(ctx, c.ServiceName)
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
	apply := &dialOption{}
	if opts != nil {
		for _, opt := range opts {
			opt(apply)
		}
	}
	// 拼接host
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
	h := apply.header
	if h != nil {
		for k, v := range h {
			request.Header.Set(k, v)
		}
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	// 塞target信息
	request.Header.Set(rpcheader.Target, c.ServiceName)
	// 去除默认User-Agent
	request.Header.Set("User-Agent", "")
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
	if respBody.StatusCode >= http.StatusBadRequest {
		return errors.New("request error with code:" + strconv.Itoa(respBody.StatusCode))
	}
	respBytes, err := io.ReadAll(respBody.Body)
	if err != nil {
		return err
	}
	if resp != nil {
		return json.Unmarshal(respBytes, resp)
	}
	return nil
}
