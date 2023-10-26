package apigw

import (
	"bytes"
	"errors"
	"github.com/LeeZXin/zsf/apigw/hexpr"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
)

const (
	mb10 = 1024 * 1024 * 10
)

type Transport struct {
	Extra           map[string]any
	rewriteStrategy RewriteStrategy
	targetSelector  hostSelector
	rpc             rpcExecutor
	config          RouterConfig
}

func (t *Transport) Transport(c *gin.Context) {
	request := c.Request
	path := request.URL.Path
	body, b := readRequestBody(c)
	if !b {
		c.String(http.StatusBadRequest, "request body error")
		return
	}
	ctx := &apiContext{
		Context: c,
		reqBody: body,
		config:  t.config,
		header:  make(http.Header),
	}
	if t.rewriteStrategy != nil {
		path = t.rewriteStrategy.Rewrite(path)
	}
	host, err := t.targetSelector.Select(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if host == "" {
		t.rpc.DoTransport(ctx, "")
	} else {
		if !strings.HasPrefix(host, "http://") {
			host = "http://" + host
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		t.rpc.DoTransport(ctx, host+path)
	}
}

func readRequestBody(ctx *gin.Context) ([]byte, bool) {
	b := make([]byte, 1024)
	ret := bytes.Buffer{}
	body := ctx.Request.Body
	defer body.Close()
	for {
		_, err := body.Read(b)
		if err == io.EOF {
			return ret.Bytes(), true
		}
		if err != nil {
			return nil, false
		}
		ret.Write(b)
		if ret.Len() > mb10 {
			return nil, false
		}
	}
}

func fullMatchTransport(routers *Routers, config RouterConfig, transport *Transport) error {
	if config.Path == "" {
		return errors.New("empty path")
	}
	routers.putFullMatchTransport(config.Path, transport)
	return nil
}

func prefixMatchTransport(routers *Routers, config RouterConfig, transport *Transport) error {
	if config.Path == "" {
		return errors.New("empty path")
	}
	routers.putPrefixMatchTransport(config.Path, transport)
	return nil
}

func exprMatchTransport(routers *Routers, config RouterConfig, transport *Transport) error {
	expr, err := hexpr.BuildExpr(config.Expr)
	if err != nil {
		return err
	}
	routers.putExprMatchTransport(&expr, transport)
	return nil
}
