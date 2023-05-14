package apigw

import (
	"github.com/LeeZXin/zsf/selector"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

type Transport struct {
	rewriteStrategy RewriteStrategy
	targetSelector  selector.Selector
	rpcExecutor     RpcExecutor
}

func (t *Transport) Transport(c *gin.Context) {
	request := c.Request
	path := request.URL.Path
	header := request.Header.Clone()
	if t.rewriteStrategy != nil {
		t.rewriteStrategy.Rewrite(&path, &header)
	}
	if t.targetSelector != nil {
		node, err := t.targetSelector.Select()
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		host := node.Data.(string)
		if !strings.HasPrefix(host, "http") {
			host = "http://" + host
		}
		t.rpcExecutor.DoTransport(c, header, host, path)
	} else {
		t.rpcExecutor.DoTransport(c, header, "", path)
	}
}
