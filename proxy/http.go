package proxy

import (
	httpclient "github.com/LeeZXin/zsf/http/client"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"time"
)

var (
	// 默认client
	httpClient = &http.Client{
		Transport: &http.Transport{
			TLSHandshakeTimeout: 10 * time.Second,
			MaxIdleConns:        20,
			IdleConnTimeout:     time.Minute,
		},
		Timeout: 30 * time.Second,
	}
)

// DoHttpProxy 实际执行http反向代理的函数
func DoHttpProxy(rpcCtx *RpcContext) error {
	ginCtx := rpcCtx.Request().(*gin.Context)
	// 获取目标服务
	serviceName := rpcCtx.TargetService()
	// 服务发现
	targetConn := httpclient.Dial(serviceName)
	// 获取服务发现ip
	targetHost, err := targetConn.Select()
	if err != nil {
		return err
	}
	url := "http://" + targetHost + ginCtx.Request.URL.RequestURI()
	logger.Logger.Info("http proxy select url: ", url)
	// 复制http.Request
	request, err := http.NewRequest(ginCtx.Request.Method, url, ginCtx.Request.Body)
	if err != nil {
		return err
	}
	header := rpcCtx.Header()
	for key := range header {
		request.Header.Set(key, header.Get(key))
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		item := strings.Builder{}
		for i, v := range vs {
			item.WriteString(v)
			if i < len(vs)-1 {
				item.WriteString(";")
			}
		}
		ginCtx.Header(k, item.String())
	}
	ginCtx.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header["Content-Type"][0], resp.Body, nil)
	return nil
}
