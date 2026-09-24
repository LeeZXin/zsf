package server

import (
	"embed"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/constants"
	"github.com/LeeZXin/zsf/http/ginutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/promhelper"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/utils/idutil"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/gin-gonic/gin"
)

// 本文件提供常用 Gin 中间件（filter）封装：
// 监控埋点、Sentinel 限流/熔断、TraceId 链路透传，以及 Vue 单页应用静态资源回退。
// 通过 AddFilters 注册到服务，或作为 WithNoRoute 处理函数使用。

// PrometheusFilter prometheus监控
// 记录每个请求的耗时与状态码（见 promhelper.HttpServerRequestTotal），
// 标签用路由模板 FullPath（未匹配为 unmatched），避免按原始 URL 维度爆炸。
// /metrics 自身的请求会被跳过，避免自监控。需配合 EnablePromApi 端点使用。
func PrometheusFilter(c *gin.Context) {
	startTime := time.Now()
	c.Next()
	path := c.FullPath()
	if path == "" {
		path = "unmatched"
	}
	promhelper.HttpServerRequestTotal(path, c.Writer.Status(), startTime)
}

// FlowLimitFilter 返回基于 Sentinel 的 QPS 限流中间件。
// 以 resource 为限流资源名（需预先在 Sentinel 规则中配置），
// 流量超限时直接返回 429 Too Many Requests。
func FlowLimitFilter(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := sentinel.Entry(resource,
			sentinel.WithResourceType(base.ResTypeWeb),
			sentinel.WithTrafficType(base.Inbound))
		if err == nil {
			defer entry.Exit()
			c.Next()
		} else {
			c.AbortWithStatus(http.StatusTooManyRequests)
		}
	}
}

// CircuitBreakerFilter 返回基于 Sentinel 的熔断中间件。
// 以 resource 为熔断资源名（需预先配置熔断规则）；熔断打开时直接返回 503 Service Unavailable。
// 请求处理期间若在 gin context 中设置了 constants.HttpInternalErr（内部错误标记，
// 见 ginutil.Error），会将该请求标记为错误请求，计入熔断器的错误率统计。
func CircuitBreakerFilter(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := sentinel.Entry(resource,
			sentinel.WithResourceType(base.ResTypeWeb),
			sentinel.WithTrafficType(base.Inbound))
		if err == nil {
			defer entry.Exit()
			c.Next()
			if c.GetBool(constants.HttpInternalErr) && entry != nil {
				entry.SetError(errors.New("internal err"))
			}
		} else {
			c.AbortWithStatus(http.StatusServiceUnavailable)
		}
	}
}

type (
	// SentinelOption SentinelFilter 的配置项函数类型
	SentinelOption  func(*sentinelOptions)
	sentinelOptions struct {
		resourceExtract func(*gin.Context) string
		blockFallback   func(*gin.Context)
	}
)

// WithResourceExtractor 自定义资源名提取函数。
// 默认资源名为 "Method:FullPath"（如 "GET:/api/user/list"）；
// 可通过该选项改为按请求头、用户等维度区分资源，便于精细化限流/熔断配置。
func WithResourceExtractor(fn func(*gin.Context) string) SentinelOption {
	return func(opts *sentinelOptions) {
		opts.resourceExtract = fn
	}
}

// WithBlockFallback 自定义请求被 Sentinel 拦截时的兜底处理函数。
// 不设置时默认按拦截类型返回：熔断 503，其余 429。
func WithBlockFallback(fn func(ctx *gin.Context)) SentinelOption {
	return func(opts *sentinelOptions) {
		opts.blockFallback = fn
	}
}

// SentinelFilter 返回可配置的 Sentinel 防护中间件（限流 + 熔断统一入口）。
// 通过 WithResourceExtractor / WithBlockFallback 定制资源名与拦截兜底；
// 被拦截时按 WithBlockFallback 或默认策略（熔断 503、其余 429）直接中止请求。
// 业务处理中标记为内部错误（constants.HttpInternalErr）的请求会计入错误率。
func SentinelFilter(opts ...SentinelOption) gin.HandlerFunc {
	sopt := &sentinelOptions{}
	for _, opt := range opts {
		opt(sopt)
	}
	return func(c *gin.Context) {
		var resource string
		if sopt.resourceExtract != nil {
			resource = sopt.resourceExtract(c)
		} else {
			resource = c.Request.Method + ":" + c.FullPath()
		}
		entry, err := sentinel.Entry(resource,
			sentinel.WithResourceType(base.ResTypeWeb),
			sentinel.WithTrafficType(base.Inbound))
		if err == nil {
			defer entry.Exit()
			c.Next()
			if c.GetBool(constants.HttpInternalErr) {
				entry.SetError(errors.New("internal err"))
			}
		} else {
			if sopt.blockFallback != nil {
				sopt.blockFallback(c)
			} else {
				switch err.BlockType() {
				case base.BlockTypeCircuitBreaking:
					c.AbortWithStatus(http.StatusServiceUnavailable)
				default:
					c.AbortWithStatus(http.StatusTooManyRequests)
				}
			}
		}
	}
}

// TraceIdFilter 负责请求级 TraceId 的透传与生成：
// 优先沿用上游请求头 rpc.TraceId，缺失则生成随机 UUID，并回写到请求头供下游 RPC 透传；
// 同时注入 context（logger.AddTraceId），使链路内日志自动携带 traceId 字段。
// 应为最外层中间件，保证后续所有中间件与业务代码都能取到 TraceId。
func TraceIdFilter(c *gin.Context) {
	traceId := c.Request.Header.Get(rpc.TraceId)
	if traceId == "" {
		traceId = idutil.RandomUUID()
		// 给newRpcHeader用
		c.Request.Header.Set(rpc.TraceId, traceId)
	}
	ctx := c.Request.Context()
	ctx = rpc.AddHeader(ctx, newRpcHeader(c))
	ctx = logger.AddTraceId(ctx, traceId)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}

func newRpcHeader(c *gin.Context) rpc.Header {
	header := make(rpc.Header, len(c.Request.Header))
	for key := range c.Request.Header {
		key = strings.ToLower(key)
		if strings.HasPrefix(key, rpc.Prefix) {
			header[key] = c.Request.Header.Get(key)
		}
	}
	return header
}

// VueFilter 返回 Vue 单页应用（SPA）的静态资源服务中间件。
// 从嵌入的 embed.FS 中按 prefix 提供前端文件；无扩展名的路径（路由跳转）
// 回退到 index.html 并禁止缓存，静态资源（带扩展名）缓存 1 小时。
// /static/、/api/、/actuator/、/metrics 前缀的请求直接放行给后续处理。
// 参数: embed - go:embed 嵌入的前端构建产物; prefix - 前端文件在 embed 中的目录前缀
func VueFilter(embed embed.FS, prefix string) gin.HandlerFunc {
	return VueFilterWithNextCondition(embed, prefix, func(c *gin.Context) bool {
		p := c.Request.URL.Path
		return strings.HasPrefix(p, "/static/") ||
			strings.HasPrefix(p, "/api/") ||
			strings.HasPrefix(p, "/metrics")
	})
}

// VueFilterWithNextCondition 与 VueFilter 相同，但放行条件由 nextCondition 自定义
// （如按项目 ID、请求头等动态判断），适用于多前端项目共存的网关场景。
// 参数: embed - 嵌入的前端构建产物; prefix - 前端文件在 embed 中的目录前缀;
// nextCondition - 返回 true 时放行给后续中间件/路由，返回 false 时走 SPA 回退逻辑
func VueFilterWithNextCondition(embed embed.FS, prefix string, nextCondition func(*gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if nextCondition(c) {
			c.Next()
			return
		}
		VueEmbedPath(c, prefix, embed)
	}
}

// VueEmbedPath 把 URL 路径映射为 embed.FS 内的斜杠路径。
// 先按 URL 根做 path.Clean，避免 .. 与绝对路径片段逃出 prefix；
// 无扩展名的路径回退 index.html。非法路径返回 ok=false。
func VueEmbedPath(c *gin.Context, prefix string, vue embed.FS) {
	cleaned := path.Clean("/" + strings.TrimPrefix(c.Request.URL.Path, "/"))
	if strings.Contains(cleaned, "..") {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var noCache bool
	rel := strings.TrimPrefix(cleaned, "/")
	if rel == "" || rel == "." || path.Ext(rel) == "" {
		rel = "index.html"
		noCache = true
	}
	prefix = path.Clean(strings.ReplaceAll(prefix, "\\", "/"))
	if prefix == "." || prefix == "/" {
		prefix = ""
	}
	if prefix == "" {
		http.ServeFileFS(c.Writer, c.Request, vue, rel)
		c.Abort()
		return
	}
	filePath := path.Join(prefix, rel)
	if filePath != prefix && !strings.HasPrefix(filePath, prefix+"/") {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if noCache {
		ginutil.NoCacheHeader(c)
	} else {
		ginutil.CacheHeader(c, time.Hour)
	}
	http.ServeFileFS(c.Writer, c.Request, vue, filePath)
	c.Abort()
	return
}
