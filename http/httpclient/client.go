package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/rpcheader"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/lb"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// client封装
// 仅支持contentType="app/json;charset=utf-8"请求发送
// 下游是http
// 支持skyWalking的传递

const (
	JsonContentType = "application/json;charset=utf-8"
)

type option struct {
	header      map[string]string
	extraHeader map[string]string
	zone        string
	discovery   discovery.Discovery
	httpClient  *http.Client
	authTs      int64
	authSecret  string
}

type Option func(*option)

func WithHeader(header map[string]string) Option {
	return func(o *option) {
		o.header = header
	}
}

func withHeader(header map[string]string) Option {
	return func(o *option) {
		o.extraHeader = header
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

func WithAuthSecret(secret string) Option {
	return func(o *option) {
		o.authSecret = secret
		o.authTs = time.Now().Unix()
	}
}

type Client interface {
	Get(ctx context.Context, path string, resp any, opts ...Option) error
	Post(ctx context.Context, path string, req, resp any, opts ...Option) error
	Put(ctx context.Context, path string, req, resp any, opts ...Option) error
	Delete(ctx context.Context, path string, resp any, opts ...Option) error
	Proxy(ctx *gin.Context, path string, opts ...Option) error
	Close()
}

type emptyClient struct{}

func (*emptyClient) Get(context.Context, string, any, ...Option) error {
	return lb.ServerNotFound
}

func (*emptyClient) Post(context.Context, string, any, any, ...Option) error {
	return lb.ServerNotFound
}

func (*emptyClient) Put(context.Context, string, any, any, ...Option) error {
	return lb.ServerNotFound
}

func (*emptyClient) Delete(context.Context, string, any, ...Option) error {
	return lb.ServerNotFound
}

func (*emptyClient) Proxy(c *gin.Context, _ string, _ ...Option) error {
	c.String(http.StatusBadGateway, lb.ServerNotFound.Error())
	return lb.ServerNotFound
}

func (*emptyClient) Close() {}

func NewEmptyClient() Client {
	return new(emptyClient)
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

func (c *clientImpl) Delete(ctx context.Context, path string, resp any, opts ...Option) error {
	err := c.send(ctx, path, http.MethodDelete, "", nil, resp, opts...)
	if err != nil {
		err = fmt.Errorf("transport: %s with err: %v", c.ServiceName, err)
	}
	return err
}

func (c *clientImpl) Proxy(ctx *gin.Context, path string, opts ...Option) error {
	req := ctx.Request
	header := make(map[string]string)
	for k := range req.Header {
		header[k] = req.Header.Get(k)
	}
	err := c.send(ctx, path, req.Method, "", req.Body, ctx, append(opts, withHeader(header))...)
	if err != nil {
		ctx.String(http.StatusBadGateway, "")
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
	// 拼接host
	url := "http://" + fmt.Sprintf("%s:%d", server.Host, server.Port)
	if !strings.HasPrefix(path, "/") {
		url += "/"
	}
	url += path
	// request
	var reqReader io.Reader
	if req != nil {
		var ok bool
		reqReader, ok = req.(io.Reader)
		if !ok {
			reqBytes, err := json.Marshal(req)
			if err != nil {
				return err
			}
			reqReader = bytes.NewReader(reqBytes)
		}
	}
	// request
	request, err := http.NewRequestWithContext(ctx, method, url, reqReader)
	if err != nil {
		return err
	}
	// 塞header
	for k, v := range opt.header {
		request.Header.Set(k, v)
	}
	for k, v := range opt.extraHeader {
		request.Header.Set(k, v)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	// 鉴权
	if opt.authSecret != "" {
		sign, err := common.GenAuthSign(opt.authSecret, opt.authTs)
		if err != nil {
			return err
		}
		request.Header.Set(rpcheader.AuthTs, strconv.FormatInt(opt.authTs, 10))
		request.Header.Set(rpcheader.AuthSign, sign)
	}
	// 塞target信息
	request.Header.Set(rpcheader.Target, c.ServiceName)
	// 去除默认User-Agent
	request.Header.Set("User-Agent", "")
	// 默认长连接去除connection: close
	request.Header.Set("Connection", "")
	// 执行拦截器
	wrapper := interceptorsWrapper{interceptorList: c.Interceptors}
	respBody, err := wrapper.intercept(request, func(request *http.Request) (*http.Response, error) {
		if opt.httpClient != nil {
			return opt.httpClient.Do(request)
		}
		return c.httpClient.Do(request)
	})
	if err != nil {
		return err
	}
	defer respBody.Body.Close()
	if resp != nil {
		if gctx, ok := resp.(*gin.Context); ok {
			for k := range respBody.Header {
				gctx.Header(k, respBody.Header.Get(k))
			}
			gctx.DataFromReader(respBody.StatusCode, respBody.ContentLength, respBody.Header.Get("Content-Type"), respBody.Body, nil)
			return nil
		}
	}
	if respBody.StatusCode != http.StatusOK {
		return fmt.Errorf("request error with code: %v", respBody.StatusCode)
	}
	if resp != nil {
		respBytes, err := io.ReadAll(respBody.Body)
		if err != nil {
			return err
		}
		return json.Unmarshal(respBytes, resp)
	}
	return nil
}
