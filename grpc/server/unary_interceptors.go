package grpcserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/header"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/skywalking"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strings"
	"time"
)

// grpc server常用拦截器封装

// headerUnaryInterceptor 请求头传递
func headerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		clone := CopyIncomingContext(ctx)
		ctx = header.SetHeaders(ctx, clone)
		ctx = logger.AppendToMDC(ctx, clone)
		return handler(ctx, req)
	}
}

func CopyIncomingContext(ctx context.Context) header.Header {
	clone := make(map[string]string, 8)
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		for key := range md {
			key = strings.ToLower(key)
			if acceptedHeaders.Contains(key) || strings.HasPrefix(key, header.Prefix) {
				val := md.Get(key)
				if val != nil && len(val) > 0 {
					clone[key] = val[0]
				}
			}
		}
	}
	_, ok = clone[header.TraceId]
	if !ok {
		clone[header.TraceId] = idutil.RandomUuid()
	}
	return clone
}

// prometheusUnaryInterceptor 监控
func prometheusUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		begin := time.Now()
		i, err := handler(ctx, req)
		prom.GrpcServerUnaryRequestTotal.
			WithLabelValues(info.FullMethod).
			Observe(float64(time.Since(begin).Milliseconds()))
		return i, err
	}
}

// logErrorUnaryInterceptor 错误信息打印
func logErrorUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		fatal := threadutil.RunSafe(func() {
			resp, err = handler(ctx, req)
			if err != nil {
				logger.Logger.WithContext(ctx).Error(err)
			}
		})
		if fatal != nil {
			logger.Logger.WithContext(ctx).Error(fatal.Error())
			err = status.Errorf(codes.Internal, "request panic")
		}
		return
	}
}

// skywalkingUnaryInterceptor 接入skywalking
func skywalkingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if skywalking.Tracer == nil {
			return handler(ctx, req)
		}
		operationName := fmt.Sprintf("GRPC %s", info.FullMethod)
		span, ctx, err := skywalking.Tracer.CreateEntrySpan(ctx, operationName, func(headerKey string) (string, error) {
			return header.GetHeaders(ctx).Get(header.PrefixForSw + headerKey), nil
		})
		if err != nil {
			return handler(ctx, req)
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOGrpcUnaryServer)
		span.Tag(skywalking.TagGrpcMethod, info.FullMethod)
		span.Tag(skywalking.TagRpcScheme, skywalking.TagGrpcScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		resp, err = handler(ctx, req)
		if err != nil {
			span.Error(time.Now(), err.Error())
		}
		return
	}
}
