package server

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/promhelper"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/utils/threadutil"

	"context"
	"strings"
	"time"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// recoverUnaryInterceptor recover封装
func recoverUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		var (
			m   any
			err error
		)
		err2 := threadutil.RunSafe(func() {
			m, err = handler(ctx, req)
		})
		if err2 == nil {
			return m, err
		}
		logger.Ctx(ctx).Error().Msgf("grpc unary panic: %v", err2)
		return nil, status.Error(codes.Internal, "internal error")
	}
}

func recoverStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == healthpb.Health_Watch_FullMethodName {
			return handler(srv, ss)
		}
		var err error
		err2 := threadutil.RunSafe(func() {
			err = handler(srv, ss)
		})
		if err2 == nil {
			return err
		}
		logger.Ctx(ss.Context()).Err(err2).Msg("grpc stream panic")
		return status.Error(codes.Internal, "internal error")
	}
}

// promUnaryInterceptor prometheus监控
func promUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		startTime := time.Now()
		m, err := handler(ctx, req)
		//耗时和频率
		promhelper.GrpcServerRequestTotal(info.FullMethod, startTime)
		return m, err
	}
}

// FlowLimitUnaryInterceptor 基于 Sentinel 的限流拦截器，resource 为限流资源名，
// 流控规则由业务侧通过 utils/sentinelutil 加载（QPS/排队等规则）。
// 被限流时直接返回 codes.ResourceExhausted，不进入 handler；未加载规则时
// 全部放行。健康检查请求（grpc_health_v1）不受限流影响。
func FlowLimitUnaryInterceptor(resource string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		entry, err := sentinel.Entry(resource,
			sentinel.WithResourceType(base.ResTypeRPC),
			sentinel.WithTrafficType(base.Inbound))
		if err == nil {
			defer entry.Exit()
			return handler(ctx, req)
		}
		return nil, status.Error(codes.ResourceExhausted, "flow limit exceeded")
	}
}

// CircuitBreakerInterceptor 基于 Sentinel 的熔断拦截器，resource 为熔断资源名，
// 熔断规则由业务侧通过 utils/sentinelutil 加载。
// handler 返回 codes.Internal/Unknown 错误时记入熔断统计（entry.SetError），
// 达到阈值后熔断期内快速失败并返回 codes.FailedPrecondition。
// 健康检查请求不受熔断影响。
func CircuitBreakerInterceptor(resource string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		entry, berr := sentinel.Entry(resource,
			sentinel.WithResourceType(base.ResTypeRPC),
			sentinel.WithTrafficType(base.Inbound))
		if berr == nil {
			defer entry.Exit()
			m, err := handler(ctx, req)
			s, ok := status.FromError(err)
			if ok && (s.Code() == codes.Internal || s.Code() == codes.Unknown) {
				entry.SetError(s.Err())
			}
			return m, err
		}
		return nil, status.Error(codes.FailedPrecondition, "circuit breaker exceeded")
	}
}

// HeaderUnaryInterceptor 传递header
func HeaderUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == healthpb.Health_Check_FullMethodName {
			return handler(ctx, req)
		}
		return handler(handleHeader(ctx), req)
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func headerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == healthpb.Health_Watch_FullMethodName {
			return handler(srv, ss)
		}
		return handler(srv, &wrappedServerStream{
			ServerStream: ss,
			ctx:          handleHeader(ss.Context()),
		})
	}
}

func handleHeader(ctx context.Context) context.Context {
	md, b := metadata.FromIncomingContext(ctx)
	if !b {
		// 应该不会走到这步
		md = metadata.New(nil)
	}
	ctx = rpc.AddHeader(ctx, copyMetadata(md))
	ctx = logger.AddTraceId(ctx, rpc.GetTraceId(ctx))
	return ctx
}

func copyMetadata(md metadata.MD) rpc.Header {
	clone := make(rpc.Header, len(md))
	for k, v := range md {
		if len(v) > 0 && strings.HasPrefix(k, rpc.Prefix) {
			clone[k] = v[0]
		}
	}
	return clone
}
