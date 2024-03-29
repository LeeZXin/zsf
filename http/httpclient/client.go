package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/rpcheader"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/lb"
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

type option struct {
	header     map[string]string
	zone       string
	discovery  discovery.Discovery
	httpClient *http.Client
}

type Option func(*option)

type headerOption struct {
	header map[string]string
}

func WithHeader(header map[string]string) Option {
	return func(o *option) {
		o.header = header
	}
}

func WithZone(zone string) Option {
	return func(o *option) {
		o.zone = zone
	}
}

func WithDiscovery(dis discovery.Discovery) Option {
	return func(o *option) {
		o.discovery = dis
	}
}

func WithHttpClient(client *http.Client) Option {
	return func(o *option) {
		o.httpClient = client
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
	httpClient   *http.Client
	Interceptors []Interceptor
}

func (c *clientImpl) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *clientImpl) Get(ctx context.Context, path string, resp any, opts ...Option) error {
	err := c.send(ctx, path, http.MethodGet, "", nil, resp, opts...)
	if err != nil {
		err = fmt.Errorf("transport: %s with err: %v", c.ServiceName, err)
	}
	return err
}
func (c *clientImpl) Post(ctx context.Context, path string, req, resp any, opts ...Option) error {
	err := c.send(ctx, path, http.MethodPost, JsonContentType, req, resp, opts...)
	if err != nil {
		err = fmt.Errorf("transport: %s with err: %v", c.ServiceName, err)
	}
	return err
}
func (c *clientImpl) Put(ctx context.Context, path string, req, resp any, opts ...Option) error {
	err := c.send(ctx, path, http.MethodPut, JsonContentType, req, resp, opts...)
	if err != nil {
		err = fmt.Errorf("transport: %s with err: %v", c.ServiceName, err)
	}
	return err
}
func (c *clientImpl) Delete(ctx context.Context, path string, req, resp any, opts ...Option) error {
	err := c.send(ctx, path, http.MethodDelete, JsonContentType, req, resp, opts...)
	if err != nil {
		err = fmt.Errorf("transport: %s with err: %v", c.ServiceName, err)
	}
	return err
}

func (c *clientImpl) send(ctx context.Context, path, method, contentType string, req, resp any, opts ...Option) error {
	// 加载选项
	opt := new(option)
	for _, apply := range opts {
		apply(opt)
	}
	// 获取服务ip
	var (
		server lb.Server
		err    error
	)
	dis := opt.discovery
	if dis == nil {
		dis = discovery.GetDefaultDiscovery()
	}
	if dis == nil {
		return errors.New("discovery is not set")
	}
	if opt.zone == "" {
		server, err = dis.ChooseServer(ctx, c.ServiceName)
		if err != nil {
			return err
		}
	} else {
		server, err = dis.ChooseServerWithZone(ctx, opt.zone, c.ServiceName)
		if err != nil {
			return err
		}
	}
	// request
	var reqBytes []byte
	if req != nil {
		reqBytes, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}
	// 拼接host
	url := "http://" + fmt.Sprintf("%s:%d", server.Host, server.Port)
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
	h := opt.header
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
		if opt.httpClient != nil {
			return opt.httpClient.Do(request)
		}
		return c.httpClient.Do(request)
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
