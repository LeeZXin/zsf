package client

import (
	"context"
	"time"

	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/promhelper"
	"github.com/LeeZXin/zsf/rpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func promUnaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	startTime := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)
	target := cc.Target()
	if target != "" {
		promhelper.GrpcClientRequestTotal(cc.Target(), method, startTime)
	}
	return err
}

func headerUnaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	headers := rpc.GetHeader(ctx)
	for k, v := range headers {
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, rpc.Source, instance.ApplicationName)
	return invoker(ctx, method, req, reply, cc, opts...)
}

// timeoutUnaryInterceptor 仅在调用方未设置 deadline 时补 60s 默认超时，
// 不缩短已有的更长 deadline（也不覆盖更短的）。
func timeoutUnaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(ctx, 60*time.Second)
		defer cancelFunc()
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

func headerStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	headers := rpc.GetHeader(ctx)
	for k, v := range headers {
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, rpc.Source, instance.ApplicationName)
	return streamer(ctx, desc, cc, method, opts...)
}
