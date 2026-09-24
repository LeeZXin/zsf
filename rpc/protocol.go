package rpc

// Protocol 服务实例的网络协议类型，用于服务发现/负载均衡按协议区分实例：
// 同一服务（application.name）可同时注册 http 与 grpc 两种协议的实例
// （见 services/registry 的 metadata 标记与 services/lb.Server.Protocol）。
type Protocol string

// 支持的协议：HttpProtocol（HTTP）与 GrpcProtocol（gRPC）。
const (
	HttpProtocol Protocol = "http"
	GrpcProtocol Protocol = "grpc"
)
