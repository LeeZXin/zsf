package apigw

import (
	"bytes"
	"net/http"
)

// rpcExecutor 请求转发执行器
type rpcExecutor interface {
	Handle(*ApiContext)
}

type mockExecutor struct {
	mockContent *MockContent
}

func (t *mockExecutor) Handle(c *ApiContext) {
	for k, v := range t.mockContent.Header {
		c.Header(k, v)
	}
	var contentType string
	switch t.mockContent.ContentType {
	case MockJsonType:
		contentType = JsonContentType
	case MockStringType:
		contentType = TextContentType
	}
	c.Data(t.mockContent.StatusCode, contentType, []byte(t.mockContent.RespStr))
}

type httpExecutor struct {
	httpClient *http.Client
}

func (t *httpExecutor) Handle(c *ApiContext) {
	if c.config.NeedAuth {
		if c.config.AuthFunc != nil {
			if !c.config.AuthFunc(c) {
				return
			}
		} else if !t.defaultAuth(c) {
			return
		}
	}
	url := c.url
	request := c.Request
	rawQuery := request.URL.RawQuery
	if rawQuery != "" {
		url = url + "?" + rawQuery
	}
	newReq, err := http.NewRequest(c.Request.Method, url, bytes.NewReader(c.reqBody))
	if err != nil {
		c.String(http.StatusInternalServerError, "")
		return
	}
	for k := range c.header {
		newReq.Header.Set(k, c.header.Get(k))
	}
	newReq.Header.Set("User-Agent", "")
	resp, err := t.httpClient.Do(newReq)
	if err != nil {
		c.String(http.StatusInternalServerError, "")
		return
	}
	defer resp.Body.Close()
	for k := range resp.Header {
		c.Header(k, resp.Header.Get(k))
	}
	c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get(ContentTypeTag), resp.Body, nil)
}
