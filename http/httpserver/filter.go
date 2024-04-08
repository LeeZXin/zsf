package httpserver

import (
	"github.com/LeeZXin/zsf-utils/collections/hashset"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/rpcheader"
	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//常见filter封装

var (
	acceptedHeaders = hashset.NewHashSet[string]()
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
			logger.Logger.WithContext(c).Error(err.Error())
			c.String(http.StatusInternalServerError, "internal error")
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
			WithLabelValues(c.Request.URL.Path, strconv.Itoa(c.Writer.Status())).
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
