package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"slices"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/llm/mcp/mcputil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// options MCP server 配置，零值表示使用默认值。
type options struct {
	Tools          []mcputil.Tool                    // 注册的工具
	MCPMiddleware  []mcp.Middleware                  // MCP 协议层中间件
	HTTPMiddleware []func(http.Handler) http.Handler // HTTP 层中间件

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	MaxBodySize int64
	Port        int

	ServerOptions         *mcp.ServerOptions
	CustomHTTPHandler     http.Handler
	StreamableHTTPOptions *mcp.StreamableHTTPOptions
	Path                  string
}

// Option MCP server 可选配置，通过 With* 函数构造。
type Option func(*options)

// WithTools 注册 MCP 工具。
func WithTools(tools ...mcputil.Tool) Option {
	return func(o *options) {
		o.Tools = tools
	}
}

// WithMCPMiddleware 注册 MCP 协议层中间件。
func WithMCPMiddleware(middleware ...mcp.Middleware) Option {
	return func(o *options) {
		o.MCPMiddleware = middleware
	}
}

// WithHTTPMiddleware 注册 HTTP 层中间件（鉴权、限流等），执行顺序与注册顺序一致。
func WithHTTPMiddleware(middleware ...func(http.Handler) http.Handler) Option {
	return func(o *options) {
		o.HTTPMiddleware = middleware
	}
}

// WithReadTimeout 设置 http.Server 读超时。
func WithReadTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.ReadTimeout = t
	}
}

// WithWriteTimeout 设置 http.Server 写超时。
func WithWriteTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.WriteTimeout = t
	}
}

// WithIdleTimeout 设置 http.Server 空闲超时。
func WithIdleTimeout(t time.Duration) Option {
	return func(opt *options) {
		opt.IdleTimeout = t
	}
}

// WithMaxBodySize 限制请求体大小，超限的请求返回 413。
// 落到 StreamableHTTPOptions.MaxRequestBodyBytes：0 表示用 SDK 默认的 4 MiB，负数表示不限制。
func WithMaxBodySize(size int64) Option {
	return func(opt *options) {
		opt.MaxBodySize = size
	}
}

// WithPort 指定监听端口，0 时取配置中心 mcp.port。
func WithPort(port int) Option {
	return func(opt *options) {
		opt.Port = port
	}
}

// WithServerOptions 透传 mcp.ServerOptions。
func WithServerOptions(serverOptions *mcp.ServerOptions) Option {
	return func(opt *options) {
		opt.ServerOptions = serverOptions
	}
}

// WithCustomHTTPHandler 完全接管 HTTP 处理（忽略内置 Streamable handler 与中间件之外的默认行为）。
func WithCustomHTTPHandler(handler http.Handler) Option {
	return func(opt *options) {
		opt.CustomHTTPHandler = handler
	}
}

// WithStreamableHTTPOptions 透传 Streamable HTTP 传输选项；不传则全用默认值。
// 可配无状态会话、JSON 响应、空闲会话超时、断线重放（EventStore）等，见 [mcp.StreamableHTTPOptions]。
func WithStreamableHTTPOptions(opts *mcp.StreamableHTTPOptions) Option {
	return func(opt *options) {
		opt.StreamableHTTPOptions = opts
	}
}

// WithPath 指定 Streamable HTTP 的响应路径，默认 /mcp。
// 用 [Server.Handler] 挂到已有引擎时，要写成引擎上注册的完整路径，否则请求会被判为路径不匹配。
func WithPath(path string) Option {
	return func(opt *options) {
		opt.Path = path
	}
}

type Server struct {
	opts       *options
	httpServer *http.Server
}

// Order 加载顺序
func (s *Server) Order() int {
	return 1
}

// build 构建 MCP server 与 HTTP handler。独立监听（OnApplicationStart）与挂到已有
// 引擎（[Server.Handler]）共用这一份，两条路径的中间件顺序与传输选项才一致。
func (s *Server) build() http.Handler {
	version := static.GetString("mcp.version")
	if version == "" {
		version = "0.0.1"
	}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: instance.ApplicationName, Version: version}, s.opts.ServerOptions)
	if len(s.opts.Tools) > 0 {
		for _, t := range s.opts.Tools {
			t.Add(mcpServer)
		}
	}
	if len(s.opts.MCPMiddleware) > 0 {
		mcpServer.AddReceivingMiddleware(s.opts.MCPMiddleware...)
	}
	// recover 兜底必须包在最外层：MCP 请求在独立 goroutine 处理，gin 的 recover 拦不住，
	// handler panic 会杀死整个进程。后 Add 的中间件先执行，这里转成协议错误。
	mcpServer.AddReceivingMiddleware(recoverMiddleware)
	handler := s.opts.CustomHTTPHandler
	if handler == nil {
		// 拷一份再改：调用方可能复用自己那份 options。
		streamableOpts := &mcp.StreamableHTTPOptions{}
		if s.opts.StreamableHTTPOptions != nil {
			*streamableOpts = *s.opts.StreamableHTTPOptions
		}
		if streamableOpts.MaxRequestBodyBytes == 0 {
			streamableOpts.MaxRequestBodyBytes = s.opts.MaxBodySize
		}
		path := s.opts.Path
		if path == "" {
			path = "/mcp"
		}
		handler = mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
			if req.URL.Path == path {
				return mcpServer
			}
			return nil
		}, streamableOpts)
	}
	if len(s.opts.HTTPMiddleware) > 0 {
		hm := s.opts.HTTPMiddleware[:]
		slices.Reverse(hm)
		for _, h := range hm {
			handler = h(handler)
		}
	}
	return handler
}

// recoverMiddleware 兜住 handler panic 并转成协议错误。
// MCP 请求在独立 goroutine 处理，宿主引擎（如 gin）的 recover 拦不住，
// 不兜住的话一个工具 panic 会杀死整个进程；这里带堆栈打日志方便定位。
func recoverMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Logger.Error().
					Str("mcpMethod", method).
					Any("panic", r).
					Bytes("stack", debug.Stack()).
					Msg("mcp handler panic")
				result = nil
				err = fmt.Errorf("internal error: handler panic")
			}
		}()
		return next(ctx, method, req)
	}
}

// Handler 返回 MCP 的 http.Handler，不监听端口，供挂到已有的 HTTP 引擎。
// 用它时不必再把这个 Server 当 lifecycle 对象启动：端口、超时与 TLS 由宿主引擎负责，
// 工具、中间件与传输选项仍按本 Server 的配置生效；路径用 [WithPath] 设成宿主上的完整路径。
// 每次调用都会新建一份 MCP server，只调一次并复用返回值。
func (s *Server) Handler() http.Handler {
	return s.build()
}

// OnApplicationStart 服务启动
func (s *Server) OnApplicationStart() {
	handler := s.build()
	port := s.opts.Port
	if port == 0 {
		port = static.GetInt("mcp.port")
	}
	if port == 0 {
		logger.Logger.Fatal().Msg("invalid mcp port")
	}
	var addr string
	host := static.GetString("mcp.host")
	if host != "" {
		addr = fmt.Sprintf("%s:%d", host, port)
	} else {
		addr = fmt.Sprintf(":%d", port)
	}
	s.httpServer = &http.Server{
		Addr:         addr,
		ReadTimeout:  s.opts.ReadTimeout,
		WriteTimeout: s.opts.WriteTimeout,
		IdleTimeout:  s.opts.IdleTimeout,
		Handler:      handler,
		ErrorLog:     log.New(io.Discard, "", 0),
	}
	go func() {
		logger.Logger.Info().Msgf("mcp server start: %v", s.httpServer.Addr)
		var err error
		err = s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Fatal().Msgf("mcp server starts failed: %v", err)
		}
	}()
}

// AfterInitialize 启动后
func (s *Server) AfterInitialize() {

}

// OnApplicationShutdown 服务关闭；超时取 mcp.shutdown-timeout，未配置默认 30s。
func (s *Server) OnApplicationShutdown() {
	if s.httpServer != nil {
		logger.Logger.Info().Msg("mcp server shutdown")
		d := quit.ShutdownTimeout(static.GetDuration("mcp.shutdown-timeout"))
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			logger.Logger.Error().Err(err).Msg("mcp server shutdown")
		}
	}
}

func NewServer(fns ...Option) *Server {
	opts := new(options)
	for _, fn := range fns {
		fn(opts)
	}
	return &Server{
		opts: opts,
	}
}
