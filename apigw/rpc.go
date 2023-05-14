package apigw

import (
	"compress/gzip"
	"encoding/json"
	"github.com/LeeZXin/zsf/logger"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
)

// RpcExecutor 请求转发执行器
type RpcExecutor interface {
	DoTransport(c *gin.Context, newHeader http.Header, target, path string)
}

type MockExecutor struct {
	MockContent *MockContent
}

func (t *MockExecutor) DoTransport(c *gin.Context, newHeader http.Header, selectHost, path string) {
	if t.MockContent.ContentType == MockJsonType {
		var m map[string]any
		err := json.Unmarshal([]byte(t.MockContent.RespStr), &m)
		if err != nil {
			c.String(http.StatusInternalServerError, "")
		} else {
			c.JSON(t.MockContent.StatusCode, m)
		}
	} else if t.MockContent.ContentType == MockStringType {
		c.String(t.MockContent.StatusCode, t.MockContent.RespStr)
	} else {
		c.String(http.StatusBadRequest, "bad request")
	}
}

type HttpExecutor struct {
}

func (t *HttpExecutor) DoTransport(c *gin.Context, newHeader http.Header, selectHost, path string) {
	request := c.Request
	rawQuery := request.URL.RawQuery
	path = selectHost + path
	if rawQuery != "" {
		path = path + "?" + rawQuery
	}
	logger.Logger.Info("rpc transport: ", path)
	newReq, err := http.NewRequest(c.Request.Method, path, request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	newReq.Header = newHeader
	resp, err := httpClient.Do(newReq)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	t.handleHttpResp(resp, c)
}

func (*HttpExecutor) handleHttpResp(resp *http.Response, c *gin.Context) {
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		item := strings.Builder{}
		for i, v := range vs {
			item.WriteString(v)
			if i < len(vs)-1 {
				item.WriteString(";")
			}
		}
		c.Header(k, item.String())
	}
	c.Header("x-gw-type", "zgw")
	if strings.Contains(c.GetHeader("Content-Encoding"), "gzip") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "error")
		} else {
			writer := gzip.NewWriter(c.Writer)
			defer writer.Close()
			if _, err := writer.Write(body); err != nil {
				c.String(http.StatusInternalServerError, "error")
			}
		}
	} else {
		c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header["Content-Type"][0], resp.Body, nil)
	}
}
