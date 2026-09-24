// Package rpc 定义 RPC 调用约定的请求头常量与透传机制（Header）。
//
// 链路约定：HTTP 侧由 http/server 的 TraceIdFilter 把 X- 开头的请求头
// （X-Account、X-Super、X-Trace-Id 等）注入 context；gRPC 侧由
// grpc/client 的 header 拦截器把 Header 写入 gRPC metadata 透传，服务端
// grpc/server 的 HeaderUnaryInterceptor 从 metadata 还原到 context。
// insession 基于其中的账号/超级管理员字段提供用户会话能力。
//
// 注意：Header 底层以 map 存储，但 Get/Set 内部把 key 统一归一化为小写，
// 因此调用方可直接使用大写常量（如 X-Account）读写；注入侧
// （newRpcHeader / copyMetadata）写入的 key 也是小写，二者一致。
// HTTP 请求头的规范命名（大小写混合）仅用于与浏览器交互，与 ctx 内的
// Header 无关。
package rpc

import (
	"context"
	"strings"

	"github.com/LeeZXin/zsf/utils/idutil"
)

// 网关/框架注入的请求头名称（HTTP 侧使用规范的大写形式；经
// Header 存储/透传时统一转为小写，Prefix 为注入侧的过滤前缀）。
const (
	TraceId  = "X-Trace-Id"
	ClientIp = "X-Client-Ip"
	Account  = "X-Account"
	Project  = "X-Project"
	Prefix   = "x-"
	Source   = "X-Source"
	Super    = "X-Super"
	UserName = "X-User-Name"
)

type headerKey struct{}

// Header 请求头集合，底层为 map[string]string（非并发安全）。
// key 统一以小写存储与查询：Get/Set 内部自动转小写，调用方可直接使用
// 大写常量（如 rpc.TraceId）；注入侧（http/server 的 newRpcHeader、
// grpc/server 的 copyMetadata）写入的 key 已是小写，二者一致。
type Header map[string]string

// Get 按 key 读取请求头值（key 自动归一化为小写），key 不存在时返回空字符串。
func (h Header) Get(key string) string {
	return h[strings.ToLower(key)]
}

// Set 设置请求头值（key 自动归一化为小写）。
func (h Header) Set(key, val string) {
	h[strings.ToLower(key)] = val
}

// GetHeader 从 ctx 中取出请求头集合；ctx 未注入过 Header 时返回空 Header
// （可直接 Set/Get，不会 panic）。
// 注意：ctx 必须非 nil（与 AddHeader/GenTraceId 不同，本函数不做 nil 兜底，
// nil context 会 panic）。
func GetHeader(ctx context.Context) Header {
	value := ctx.Value(headerKey{})
	if value != nil {
		return value.(Header)
	}
	return make(Header)
}

// GetTraceId 从 ctx 中读取 traceId；Header 已注入但尚未有 TraceId 时生成一次
// 并写回该 Header，同一 ctx 后续读取稳定。ctx 未注入 Header 时返回新 UUID
// （无法写回，仅作兜底，链路入口应先 AddHeader / GenTraceId）。
func GetTraceId(ctx context.Context) string {
	value := ctx.Value(headerKey{})
	if value == nil {
		return idutil.RandomUUID()
	}
	h := value.(Header)
	t := h.Get(TraceId)
	if t == "" {
		t = idutil.RandomUUID()
		h.Set(TraceId, t)
	}
	return t
}

// AddHeader 将 header 合并进 ctx：
//   - ctx 已注入过 Header：原地合并修改并返回原 ctx（不创建新 context 节点）
//   - ctx 未注入过：创建新 Header 并返回新 ctx
//
// 注意：原地修改意味着多个调用方共享同一个 Header map，并发写入会数据竞争；
// 需要独立副本时请自行拷贝（如 grpc/server 的 copyMetadata）。
// ctx 为 nil 时自动改用 context.Background()。
func AddHeader(ctx context.Context, header Header) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if v := ctx.Value(headerKey{}); v != nil {
		// Header 已存在，直接原地修改，不创建新 context
		h := v.(Header)
		for k, v := range header {
			h.Set(k, v)
		}
		return ctx
	}
	// 首次创建 Header
	h := make(Header, len(header))
	for k, v := range header {
		h.Set(k, v)
	}
	return context.WithValue(ctx, headerKey{}, h)
}

// GenTraceId 生成新的 UUID 作为 traceId 注入 ctx，返回新 ctx 与 traceId。
// 通常用于网关/入口处为请求生成链路标识（http/server 的 TraceIdFilter
// 在请求头缺失 traceId 时也会生成）；ctx 为 nil 时自动改用 context.Background()。
func GenTraceId(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	uuid := idutil.RandomUUID()
	return AddHeader(ctx, map[string]string{
		TraceId: uuid,
	}), uuid
}
