package httpserver

import (
	"fmt"
	"github.com/LeeZXin/zsf-utils/collections/hashset"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/rpcheader"
	"github.com/LeeZXin/zsf/skywalking"
	"github.com/SkyAPM/go2sky"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/gin-gonic/gin"
	"net/http"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strconv"
	"strings"
	"time"
)

//常见filter封装

var (
	acceptedHeaders = hashset.NewHashSet[string](nil)
)

func init() {
	h := static.GetString("http.server.acceptedHeaders")
	if h != "" {
		sp := strings.Split(h, ";")
		for _, s := range sp {
			acceptedHeaders.Add(s)
		}
	}
}

// recoverFilter recover封装
func recoverFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := threadutil.RunSafe(func() {
			c.Next()
		})
		if err != nil {
			logger.Logger.WithContext(c.Request.Context()).Error(err.Error())
			c.String(500, "系统异常,稍后重试")
			c.Abort()
		}
	}
}

// promFilter prometheus监控
func promFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		begin := time.Now()
		c.Next()
		//耗时和频率
		prom.HttpServerRequestTotal.
			WithLabelValues(c.Request.URL.Path).
			Observe(float64(time.Since(begin).Milliseconds()))
	}
}

func WithSentinel(resource string, invoke gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := sentinel.Entry(resource, sentinel.WithTrafficType(base.Inbound))
		if err == nil {
			defer entry.Exit()
			invoke(c)
		} else {
			c.String(http.StatusTooManyRequests, "request limit")
			c.Abort()
		}
	}
}

// headerFilter 传递header
func headerFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Header.Get(rpcheader.TraceId) == "" {
			c.Request.Header.Set(rpcheader.TraceId, idutil.RandomUuid())
		}
		clone := CopyRequestHeader(c)
		ctx := rpcheader.SetHeaders(c.Request.Context(), clone)
		ctx = logger.AppendToMDC(ctx, map[string]string{
			logger.TraceId: clone.Get(rpcheader.TraceId),
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func CopyRequestHeader(c *gin.Context) rpcheader.Header {
	clone := make(rpcheader.Header, len(c.Request.Header))
	for key := range c.Request.Header {
		key = strings.ToLower(key)
		if acceptedHeaders.Contains(key) || strings.HasPrefix(key, rpcheader.Prefix) {
			clone[key] = c.Request.Header.Get(key)
		}
	}
	return clone
}

func skywalkingFilter() gin.HandlerFunc {
	if skywalking.Tracer == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		operationName := fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL)
		span, ctx, err := skywalking.Tracer.CreateEntrySpan(c.Request.Context(), operationName, func(headerKey string) (string, error) {
			return c.Request.Header.Get(rpcheader.PrefixForSw + headerKey), nil
		})
		if err != nil {
			logger.Logger.WithContext(c.Request.Context()).Error(err)
			c.Next()
			return
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOHttpServer)
		span.Tag(go2sky.TagHTTPMethod, c.Request.Method)
		span.Tag(go2sky.TagURL, c.Request.URL.Path)
		span.Tag(skywalking.TagRpcScheme, skywalking.TagHttpScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		if len(c.Errors) > 0 {
			span.Error(time.Now(), c.Errors.String())
		}
		span.Tag(go2sky.TagStatusCode, strconv.Itoa(c.Writer.Status()))
	}
}
