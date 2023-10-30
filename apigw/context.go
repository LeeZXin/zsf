package apigw

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type ApiContext struct {
	*gin.Context
	reqBody []byte
	config  RouterConfig
	header  http.Header
	url     string
}

func (c *ApiContext) ReqBody() []byte {
	return c.reqBody
}

func (c *ApiContext) ReqHeader() http.Header {
	return c.header
}

func (c *ApiContext) Config() RouterConfig {
	return c.config
}

func (c *ApiContext) Url() string {
	return c.url
}
