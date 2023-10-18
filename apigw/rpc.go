package apigw

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
)

// RpcExecutor 请求转发执行器
type RpcExecutor interface {
	DoTransport(c *gin.Context, newHeader http.Header, target, path string)
}

type mockExecutor struct {
	mockContent MockContent
}

func (t *mockExecutor) DoTransport(c *gin.Context, newHeader http.Header, selectHost, path string) {
	headersStr := t.mockContent.Headers
	if headersStr != "" {
		var h map[string]string
		err := json.Unmarshal([]byte(headersStr), &h)
		if err == nil {
			for k, v := range h {
				c.Header(k, v)
			}
		}
	}
	switch t.mockContent.ContentType {
	case MockJsonType:
		var m map[string]any
		err := json.Unmarshal([]byte(t.mockContent.RespStr), &m)
		if err != nil {
			c.String(http.StatusInternalServerError, "")
		} else {
			c.JSON(t.mockContent.StatusCode, m)
		}
	case MockStringType:
		c.String(t.mockContent.StatusCode, t.mockContent.RespStr)
	default:
		c.String(http.StatusBadRequest, "bad request")
	}
}

type httpExecutor struct {
	httpClient *http.Client
}

func (t *httpExecutor) DoTransport(c *gin.Context, newHeader http.Header, selectHost, path string) {
	request := c.Request
	rawQuery := request.URL.RawQuery
	path = selectHost + path
	if rawQuery != "" {
		path = path + "?" + rawQuery
	}
	newReq, err := http.NewRequest(c.Request.Method, path, request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	newReq.Header = newHeader
	resp, err := t.httpClient.Do(newReq)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	t.handleHttpResp(resp, c)
}

func (*httpExecutor) handleHttpResp(resp *http.Response, c *gin.Context) {
	defer resp.Body.Close()
	for k := range resp.Header {
		c.Header(k, resp.Header.Get(k))
	}
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}
