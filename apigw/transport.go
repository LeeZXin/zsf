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

type Transport interface {
	Handle(*gin.Context)
}

type transportImpl struct {
	rewriteStrategy RewriteStrategy
	targetSelector  hostSelector
	rpc             rpcExecutor
	config          RouterConfig
}

func (t *transportImpl) Handle(c *gin.Context) {
	request := c.Request
	path := request.URL.Path
	body, b := readRequestBody(c)
	if !b {
		c.String(http.StatusBadRequest, "request body error")
		return
	}
	if t.rewriteStrategy != nil {
		path = t.rewriteStrategy.Rewrite(path)
	}
	host, err := t.targetSelector.Select(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	ctx := &apiContext{
		Context: c,
		reqBody: body,
		config:  t.config,
		header:  make(http.Header),
	}
	if host != "" {
		if !strings.HasPrefix(host, "http://") {
			host = "http://" + host
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		ctx.url = host + path
	}
	t.rpc.Handle(ctx)
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

func fullMatchTransport(r *Routers, c RouterConfig, t Transport) error {
	if c.Path == "" {
		return errors.New("empty path")
	}
	r.putFullMatchTransport(c.Path, t)
	return nil
}

func prefixMatchTransport(r *Routers, c RouterConfig, t Transport) error {
	if c.Path == "" {
		return errors.New("empty path")
	}
	r.putPrefixMatchTransport(c.Path, t)
	return nil
}

func exprMatchTransport(r *Routers, c RouterConfig, t Transport) error {
	expr, err := hexpr.BuildExpr(c.Expr)
	if err != nil {
		return err
	}
	r.putExprMatchTransport(&expr, t)
	return nil
}
