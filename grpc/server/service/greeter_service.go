// Package service 提供 gRPC 业务服务实现的示例（demo）代码，
// 演示服务端接入方式：实现 .proto 生成的服务接口并通过
// grpc/server 的 RegisterService 注册；日志使用 logger.Ctx 携带 traceId。
// 业务服务可参照本示例接入，也可直接删除本包。
package service

import (
	"github.com/LeeZXin/zsf/grpc/grpcutil"
	"github.com/LeeZXin/zsf/grpc/server/testproto"
	"github.com/LeeZXin/zsf/logger"

	"context"
)

// GreeterService 是 gRPC 服务示例实现（对应 testproto.GreeterServiceServer），
// Port 仅用于日志演示区分多实例。
type GreeterService struct {
	Port int
	testproto.GreeterServiceServer
}

// HelloWorld 处理 HelloReq 请求：打印入参并固定返回 "you"（纯演示逻辑）。
func (s *GreeterService) HelloWorld(ctx context.Context, req *testproto.HelloReq) (*testproto.HelloResp, error) {
	reqStr, _ := grpcutil.MessageToString(req)
	logger.Ctx(ctx).Info().Msgf("GreeterService.HelloWorld port: %v, req: %v", s.Port, reqStr)
	return &testproto.HelloResp{
		Code:    0,
		Message: "you",
	}, nil
}
