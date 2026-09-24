// Package server 提供基于 Gin 的 HTTP 服务封装，负责创建和管理 HTTP 服务生命周期。
//
// 通过 NewDefaultServer + Option 构建服务实例，并作为 lifecycle.Object 接入框架启动流程：
//   - OnApplicationStart：构建 Gin 引擎（注册路由、中间件、静态资源等）并启动监听，
//     端口未配置时回退读取静态配置 http.port；端口非法、HTTPS 证书缺失等直接 Fatal
//   - AfterInitialize：服务已可访问后，向注册中心注册（注册失败 Fatal）
//   - OnApplicationShutdown：健康检查摘流 → 注销注册中心 → 优雅关闭 HTTP 监听
//
// 职责边界：本包只负责 HTTP 层（监听、路由、中间件、静态文件），
// 请求参数绑定与统一响应格式由 http/ginutil 提供，业务错误由 http/bizerr 定义。
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/constants"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/registry"

	"github.com/VictoriaMetrics/metrics"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

const (
	// MetricsPath 是 Prometheus 监控指标的 HTTP 端点路径（/metrics）。
	// 仅启用 EnablePromApi 选项时注册；埋点方需保证指标名一致（见 promhelper 包）。
	MetricsPath = "/metrics"

	// actuatorDrainWait 是 /api/actuator/health 改为 503 之后、真正 Shutdown 之前的等待。
	// nginx 主动健康检查（interval × fails）或 max_fails 需要若干探测周期才会把实例标 down；
	// 若立刻关监听，探测窗口内仍可能打到本机并 connection refused，对外表现为 502。
	actuatorDrainWait = 5 * time.Second
)

// Server HTTP 服务封装，负责创建和管理 Gin 引擎及 HTTP 服务生命周期。
type Server struct {
	opts       *options           // 服务配置选项
	httpServer *http.Server       // 底层 HTTP 服务实例
	registrar  registry.Registrar // 服务注册
	// isDraining 进程退出时置位，使 /api/actuator/health 返回 503。
	// 语义是 draining（连接排空 / 摘流），不是 inShutdown：此时监听仍在、
	// 存量请求继续处理，只是让 nginx / 负载均衡把本实例从 upstream 摘掉，
	// 不再转发新流量。atomic 因为探活与 OnApplicationShutdown 并发。
	isDraining atomic.Bool
}

// options HTTP 服务配置选项，包含端口、超时、HTTPS、中间件等设置
type options struct {
	noRoute  gin.HandlerFunc   // 未匹配路由时的处理函数
	noMethod gin.HandlerFunc   // 请求方法不允许时的处理函数
	routers  []gin.OptionFunc  // 路由注册函数列表
	filters  []gin.HandlerFunc // 中间件列表

	host         string // HTTP 监听host
	httpPort     int    // HTTP 监听端口
	enableHttps  bool   // 是否启用 HTTPS
	certFilePath string // HTTPS 证书文件路径
	keyFilePath  string // HTTPS 私钥文件路径
	// getCertificate 动态证书回调：设置后 TLS 证书按握手从回调获取（支持热替换、
	// 按 SNI 分派多证书），证书文件路径无需指定（见 WithGetCertificate）。
	getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)

	readTimeout  time.Duration // 请求读取超时时间
	writeTimeout time.Duration // 响应写入超时时间
	idleTimeout  time.Duration // 空闲连接超时时间

	disableUseH2C  bool // 是否禁用 H2C（HTTP/2 over cleartext）
	enableActuator bool // 是否启用 Actuator 健康检查端点
	enablePromApi  bool // 是否启用 Prometheus 监控指标端点

	enablePprof bool // 是否启动pprof

	htmlGlob    string // HTML 模板文件匹配路径
	maxBodySize int64  // 最大请求体大小（字节）

	disableGzip       bool     // 是否禁用 Gzip 压缩
	gzipExcludeRegexs []string // Gzip 排除路径正则（如 git smart HTTP）

	enableStatic bool // 是否启用静态文件服务

	newRegistrarFunc registry.NewRegistrarFunc

	name string
}

// Option 服务配置选项函数类型
type Option func(*options)

// WithHttpPort 设置 HTTP 监听端口
func WithHttpPort(port int) Option {
	return func(opt *options) {
		opt.httpPort = port
	}
}

// WithMaxBodySize 设置最大请求体大小（字节）
func WithMaxBodySize(size int64) Option {
	return func(opt *options) {
		opt.maxBodySize = size
	}
}

// WithNewRegistrarFunc 设置自定义的服务注册中心工厂函数。
// 默认不注册服务到注册中心；传入后，启动流程会自动创建并注册/注销（见 AfterInitialize 与 OnApplicationShutdown）。
// 注册失败会 Fatal 退出。
func WithNewRegistrarFunc(f registry.NewRegistrarFunc) Option {
	return func(opt *options) {
		opt.newRegistrarFunc = f
	}
}

// WithName 设置服务名称，仅用于日志标识（如 "http server: xxx starts"）。
func WithName(name string) Option {
	return func(opt *options) {
		opt.name = name
	}
}

// WithHost 设置 HTTP 监听地址（host:port 中的 host 部分）。
// 不传时监听所有网卡（":port"）。
func WithHost(host string) Option {
	return func(opt *options) {
		opt.host = host
	}
}

// EnableActuator 启用 Actuator 端点：/api/actuator/health（存活/摘流探活）和 /api/actuator/gc。
// health 供 nginx 等负载均衡做主动健康检查：正常 200，优雅退出进入 draining 后 503。
func EnableActuator() Option {
	return func(opt *options) {
		opt.enableActuator = true
	}
}

// EnablePromApi 启用 Prometheus 监控指标端点（/metrics）
func EnablePromApi() Option {
	return func(opt *options) {
		opt.enablePromApi = true
	}
}

// DisableGzip 禁用 Gzip 响应压缩
func DisableGzip() Option {
	return func(opt *options) {
		opt.disableGzip = true
	}
}

// WithGzipExcludeRegex 排除匹配正则的路径，不压缩响应。
// git smart HTTP 的 pkt-line 被 gzip 后客户端无法解析。
func WithGzipExcludeRegex(patterns ...string) Option {
	return func(opt *options) {
		opt.gzipExcludeRegexs = append(opt.gzipExcludeRegexs, patterns...)
	}
}

// EnablePProf 启用 pprof 性能分析端点（/api/debug/pprof/*）。
// 仅建议在排查线上问题时开启，生产环境常驻会带来额外开销。
func EnablePProf() Option {
	return func(opt *options) {
		opt.enablePprof = true
	}
}

// EnableStatic 启用静态文件服务（默认目录 /static）
func EnableStatic() Option {
	return func(opt *options) {
		opt.enableStatic = true
	}
}

// RegisterRouter 注册 HTTP 路由
func RegisterRouter(routers ...gin.OptionFunc) Option {
	return func(opt *options) {
		opt.routers = routers
	}
}

// AddFilters 添加 Gin 中间件
func AddFilters(filters ...gin.HandlerFunc) Option {
	return func(opt *options) {
		opt.filters = append(opt.filters, filters...)
	}
}

// WithNoRoute 设置未匹配路由时的处理函数
func WithNoRoute(f gin.HandlerFunc) Option {
	return func(opt *options) {
		opt.noRoute = f
	}
}

// WithNoMethod 设置请求方法不允许时的处理函数
func WithNoMethod(f gin.HandlerFunc) Option {
	return func(opt *options) {
		opt.noMethod = f
	}
}

// WithReadTimeout 设置请求读取超时时间
func WithReadTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.readTimeout = t
	}
}

// WithWriteTimeout 设置响应写入超时时间
func WithWriteTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.writeTimeout = t
	}
}

// WithIdleTimeout 设置空闲连接超时时间
func WithIdleTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.idleTimeout = t
	}
}

// WithDisableUseH2C 禁用 H2C（HTTP/2 over cleartext）
func WithDisableUseH2C() Option {
	return func(opt *options) {
		opt.disableUseH2C = true
	}
}

// EnableHttps 启用 HTTPS 并指定证书和私钥文件路径
func EnableHttps(certFilePath, keyFilePath string) Option {
	return func(opt *options) {
		opt.enableHttps = true
		opt.certFilePath = certFilePath
		opt.keyFilePath = keyFilePath
	}
}

// WithGetCertificate 启用动态证书 HTTPS 模式：TLS 证书由回调按握手提供，
// 配合 zsf/http/autohttps 等使用，支持证书热替换与多证书按 SNI 分派。
// 设置后即视为启用 HTTPS，无需指定证书文件；与 EnableHttps 同时指定
// 证书文件会冲突（OnApplicationStart 时 Fatal）。
func WithGetCertificate(fn func(*tls.ClientHelloInfo) (*tls.Certificate, error)) Option {
	return func(opt *options) {
		opt.enableHttps = true
		opt.getCertificate = fn
	}
}

// WithHtmlGlob 设置 HTML 模板文件的匹配路径
func WithHtmlGlob(h string) Option {
	return func(opt *options) {
		opt.htmlGlob = h
	}
}

// Order 返回服务启动顺序（优先级），0 表示默认优先级
func (s *Server) Order() int {
	return 0
}

// OnApplicationStart 实现 lifecycle.Object 接口，在生命周期启动阶段构建并启动 HTTP 服务。
//
// 执行内容：创建 Gin 引擎（ReleaseMode）并依次装配 H2C、静态资源、
// 404/405 处理、HTML 模板、请求体大小限制、Gzip、自定义中间件、各功能端点与业务路由，
// 最后在独立 goroutine 中启动监听。
//
// 注意：
//   - 监听端口优先取 WithHttpPort，未配置时回退静态配置 http.port；两者都无效则 Fatal
//   - HTTPS 启用时证书/私钥路径为空或资源缺失直接 Fatal（fail-fast，不带病启动）
//   - 监听启动失败（端口被占用等）同样 Fatal
func (s *Server) OnApplicationStart() {
	//gin mode
	gin.SetMode(gin.ReleaseMode)
	//create gin
	engine := gin.New()
	if !s.opts.disableUseH2C {
		engine.UseH2C = true
	}
	engine.MaxMultipartMemory = 32 << 20
	engine.ContextWithFallback = true
	if s.opts.enableStatic {
		//静态资源文件路径
		engine.Static("/static", filepath.Join(constants.ResourcesDir, "static"))
	}
	// 404
	if s.opts.noRoute != nil {
		engine.NoRoute(s.opts.noRoute)
	}
	if s.opts.noMethod != nil {
		engine.NoMethod(s.opts.noMethod)
	} else if s.opts.noRoute != nil {
		engine.NoMethod(s.opts.noRoute)
	}
	if s.opts.htmlGlob != "" {
		engine.LoadHTMLGlob(s.opts.htmlGlob)
	}
	if s.opts.maxBodySize > 0 {
		engine.Use(func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.opts.maxBodySize)
			c.Next()
		})
	}
	if !s.opts.disableGzip {
		gzOpts := []gzip.Option{gzip.WithDecompressFn(gzip.DefaultDecompressHandle)}
		if len(s.opts.gzipExcludeRegexs) > 0 {
			gzOpts = append(gzOpts, gzip.WithExcludedPathsRegexs(s.opts.gzipExcludeRegexs))
		}
		engine.Use(gzip.Gzip(gzip.DefaultCompression, gzOpts...))
	}
	// 注册自定义中间件
	if len(s.opts.filters) > 0 {
		engine.Use(s.opts.filters...)
	}
	// actuator
	if s.opts.enableActuator {
		logger.Logger.Info().Msgf("http server: %s enables actuator", s.opts.name)
		s.enableActuator(engine)
	}
	// prom api
	if s.opts.enablePromApi {
		logger.Logger.Info().Msgf("http server: %s enables prometheus api", s.opts.name)
		s.enablePromApi(engine)
	}
	// pprof api
	if s.opts.enablePprof {
		logger.Logger.Info().Msgf("http server: %s enables pprof", s.opts.name)
		s.enablePprof(engine)
	}
	// router
	for _, router := range s.opts.routers {
		router(engine)
	}
	httpPort := s.opts.httpPort
	if httpPort <= 0 {
		httpPort = static.GetInt("http.port")
	}
	if httpPort <= 0 {
		logger.Logger.Fatal().Msgf("http server: %s port: %d is invalid", s.opts.name, httpPort)
	}
	var addr string
	host := s.opts.host
	if host != "" {
		addr = fmt.Sprintf("%s:%d", host, httpPort)
	} else {
		addr = fmt.Sprintf(":%d", httpPort)
	}
	s.httpServer = &http.Server{
		Addr:         addr,
		ReadTimeout:  s.opts.readTimeout,
		WriteTimeout: s.opts.writeTimeout,
		IdleTimeout:  s.opts.idleTimeout,
		Handler:      engine.Handler(),
		ErrorLog:     log.New(io.Discard, "", 0),
	}
	var (
		certFilePath string
		keyFilePath  string
	)
	if s.opts.enableHttps {
		if s.opts.getCertificate != nil {
			// 动态证书模式（如 autohttps）：证书由回调按握手提供，文件路径允许为空。
			// NextProtos 附 acme-tls/1 以支持 ACME TLS-ALPN-01 挑战；普通客户端
			// 不会 offer 该协议，无副作用。h2/http/1.1 由 stdlib 自动补齐。
			if s.opts.certFilePath != "" || s.opts.keyFilePath != "" {
				logger.Logger.Fatal().Msgf("http server: %s WithGetCertificate conflicts with EnableHttps cert/key files", s.opts.name)
			}
			s.httpServer.TLSConfig = &tls.Config{
				GetCertificate: s.opts.getCertificate,
				NextProtos:     []string{"h2", "http/1.1", "acme-tls/1"},
				MinVersion:     tls.VersionTLS12,
			}
		} else {
			certFilePath = s.opts.certFilePath
			keyFilePath = s.opts.keyFilePath
			if certFilePath == "" {
				logger.Logger.Fatal().Msgf("http server: %s https.certFile is empty", s.opts.name)
			} else {
				certFilePath = filepath.Join(constants.ResourcesDir, certFilePath)
			}
			if keyFilePath == "" {
				logger.Logger.Fatal().Msgf("http server: %s https.keyFile is empty", s.opts.name)
			} else {
				keyFilePath = filepath.Join(constants.ResourcesDir, keyFilePath)
			}
		}
	}
	if s.opts.newRegistrarFunc != nil {
		var err error
		s.registrar, err = s.opts.newRegistrarFunc(rpc.HttpProtocol, httpPort)
		if err != nil {
			logger.Logger.Fatal().Msgf("http server: %s failed to register registrar: %v", s.opts.name, err)
		}
	}
	go func() {
		var err error
		if s.opts.enableHttps {
			logger.Logger.Info().Msgf("http server: %s start: %v", s.opts.name, s.httpServer.Addr)
			logger.Logger.Info().Msgf("http server: %s certFile path: %s", s.opts.name, certFilePath)
			logger.Logger.Info().Msgf("http server: %s keyFile path: %s", s.opts.name, keyFilePath)
			err = s.httpServer.ListenAndServeTLS(certFilePath, keyFilePath)
		} else {
			logger.Logger.Info().Msgf("http server: %s start: %v", s.opts.name, s.httpServer.Addr)
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Fatal().Msgf("http server: %s starts failed: %v", s.opts.name, err)
		}
	}()
}

func (s *Server) enableActuator(e *gin.Engine) {
	// nginx / 负载均衡探活。只看 HTTP 状态码：200 在池，503 摘流。
	// 用 503 而不是关端口，避免探活失败窗口内 connection refused → 502。
	e.Any("/api/actuator/health", func(c *gin.Context) {
		if s.isDraining.Load() {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		c.String(http.StatusOK, "")
	})
	// 触发gc
	e.Any("/api/actuator/gc", func(c *gin.Context) {
		go runtime.GC()
		c.String(http.StatusOK, "")
	})
}

func (s *Server) enablePprof(r *gin.Engine) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	r.Any("/api/debug/pprof/*any", gin.WrapH(http.StripPrefix("/api", mux)))
}

func (s *Server) enablePromApi(r *gin.Engine) {
	r.Any(MetricsPath, func(c *gin.Context) {
		metrics.WritePrometheus(c.Writer, true)
	})
}

// AfterInitialize 实现 lifecycle.Object 接口，在所有对象 OnApplicationStart 完成后执行。
// 若配置了注册中心（WithNewRegistrarFunc），此处将服务注册到注册中心；注册失败 Fatal。
func (s *Server) AfterInitialize() {
	if s.registrar != nil {
		err := s.registrar.Register()
		if err != nil {
			logger.Logger.Fatal().Msgf("http server: %s failed to register registrar: %v", s.opts.name, err)
		}
	}
}

// OnApplicationShutdown 实现 lifecycle.Object 接口，进程退出时执行优雅关闭。
//
// 顺序对齐「先停新流量、再释放监听」：
//  1. 置 isDraining：health 立刻 503，nginx 摘流（须先于 Deregister；注销有网络 RTT）
//  2. 注销注册中心：服务发现客户端不再选本实例
//  3. 等待 actuatorDrainWait：给 nginx 一个探测周期真正把实例标 down
//  4. Shutdown：停监听并等待存量请求；超时取 http.shutdown-timeout，未配置默认 30s
func (s *Server) OnApplicationShutdown() {
	s.isDraining.Store(true)
	if s.registrar != nil {
		s.registrar.Deregister()
	}
	if s.opts.enableActuator {
		time.Sleep(actuatorDrainWait)
	}
	if s.httpServer != nil {
		logger.Logger.Info().Msgf("http server: %s shutdown", s.opts.name)
		d := quit.ShutdownTimeout(static.GetDuration("http.shutdown-timeout"))
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			logger.Logger.Error().Err(err).Msgf("http server: %s shutdown", s.opts.name)
		}
	}
}

// GetHttpServer 返回底层 *http.Server 实例。
// 仅在 OnApplicationStart 执行之后非 nil，此前调用返回 nil。
func (s *Server) GetHttpServer() *http.Server {
	return s.httpServer
}

// panicRecovery 是 gin Recovery 的回调，用项目 logger 记录真实 panic。
// 客户端断连类 panic（http.ErrAbortHandler、broken pipe）由 gin 内部静默处理，不会走到这里。
func panicRecovery(c *gin.Context, rec any) {
	logger.Ctx(c.Request.Context()).Error().
		Interface("panic", rec).
		Str("stack", string(debug.Stack())).
		Msg("panic recovered")
	c.String(http.StatusInternalServerError, "internal error")
	c.Abort()
}

// NewDefaultServer 创建 HTTP 服务实例，默认内置 panic 恢复中间件（记录真实 panic 并返回 500）。
// 通过 fns 传入各 Option 定制行为；通常配合 lifecycle.WithObjects 使用。
func NewDefaultServer(fns ...Option) *Server {
	defaultOpts := []Option{
		AddFilters(gin.CustomRecoveryWithWriter(nil, panicRecovery)),
	}
	fns = append(defaultOpts, fns...)
	opts := new(options)
	for _, fn := range fns {
		fn(opts)
	}
	return &Server{
		opts: opts,
	}
}

func ApiOnDev(api ...gin.OptionFunc) gin.OptionFunc {
	return func(e *gin.Engine) {
		if instance.IsDev {
			for _, a := range api {
				a(e)
			}
		}
	}
}
