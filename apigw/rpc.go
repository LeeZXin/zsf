package apigw

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// rpcExecutor 请求转发执行器
type rpcExecutor interface {
	DoTransport(*apiContext, string)
}

type mockExecutor struct {
	mockContent MockContent
}

func (t *mockExecutor) DoTransport(c *apiContext, _ string) {
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

func (t *httpExecutor) DoTransport(c *apiContext, url string) {
	if c.config.NeedAuth && !auth(c) {
		return
	}
	request := c.Request
	rawQuery := request.URL.RawQuery
	if rawQuery != "" {
		url = url + "?" + rawQuery
	}
	newReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(c.reqBody))
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	resp, err := t.httpClient.Do(newReq)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()
	for k := range resp.Header {
		c.Header(k, resp.Header.Get(k))
	}
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}
