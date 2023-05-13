package httpserver

import (
	"fmt"
	"github.com/SkyAPM/go2sky"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"runtime"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strconv"
	"strings"
	"time"
	"zsf/logger"
	"zsf/prom"
	"zsf/rpc"
	"zsf/skywalking"
)

//常见filter封装

// RecoverFilter recover封装
func RecoverFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			fatal := recover()
			if fatal != nil {
				stack := make([]string, 0, 20)
				for i := 0; i < 20; i++ {
					_, file, line, ok := runtime.Caller(i)
					if !ok {
						break
					}
					stack = append(stack, file+":"+strconv.Itoa(line))
				}
				logger.Logger.WithContext(c.Request.Context()).Error(fatal, "\n", strings.Join(stack, "\n"))
				c.String(500, "系统异常,稍后重试")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// PromFilter prometheus监控
func PromFilter() gin.HandlerFunc {
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
			c.String(http.StatusForbidden, "request limit")
			c.Abort()
		}
	}
}

// HeaderFilter 传递header
func HeaderFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Header.Get(rpc.TraceId) == "" {
			c.Request.Header.Set(rpc.TraceId, strings.ReplaceAll(uuid.New().String(), "-", ""))
		}
		clone := make(map[string]string, len(c.Request.Header))
		for key := range c.Request.Header {
			key = strings.ToLower(key)
			if strings.HasPrefix(strings.ToLower(key), rpc.Prefix) {
				clone[key] = c.Request.Header.Get(key)
			}
		}
		ctx := rpc.AppendToHeader(c.Request.Context(), clone)
		ctx = logger.AppendToMDC(ctx, clone)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func SkyWalkingFilter() gin.HandlerFunc {
	if skywalking.Tracer == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		operationName := fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL)
		span, ctx, err := skywalking.Tracer.CreateEntrySpan(c.Request.Context(), operationName, func(headerKey string) (string, error) {
			return c.Request.Header.Get(rpc.PrefixForSw + headerKey), nil
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
