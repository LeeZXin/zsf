package apigw

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

type Transport struct {
	Extra           map[string]any
	rewriteStrategy RewriteStrategy
	targetSelector  Selector
	rpcExecutor     RpcExecutor
}

func (t *Transport) Transport(c *gin.Context) {
	request := c.Request
	path := request.URL.Path
	header := request.Header.Clone()
	if t.rewriteStrategy != nil {
		t.rewriteStrategy.Rewrite(&path, header)
	}
	if t.targetSelector != nil {
		host, err := t.targetSelector.Select(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		t.rpcExecutor.DoTransport(c, header, host, path)
	} else {
		t.rpcExecutor.DoTransport(c, header, "", path)
	}
}
