package grpcserver

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"runtime"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strconv"
	"strings"
	"time"
	"zsf/logger"
	"zsf/prom"
	"zsf/rpc"
	"zsf/skywalking"
)

// grpc server常用拦截器封装

// headerUnaryInterceptor 请求头传递
func headerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		clone := copyIncomingContext(ctx)
		ctx = rpc.AppendToHeader(ctx, clone)
		ctx = logger.AppendToMDC(ctx, clone)
		return handler(ctx, req)
	}
}

func copyIncomingContext(ctx context.Context) rpc.Header {
	clone := make(map[string]string, 8)
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		for key, val := range md {
			key = strings.ToLower(key)
			if strings.HasPrefix(key, rpc.Prefix) {
				v := ""
				if val != nil && len(val) > 0 {
					v = val[0]
				}
				clone[key] = v
			}
		}
	}
	_, ok = clone[rpc.TraceId]
	if !ok {
		clone[rpc.TraceId] = strings.ReplaceAll(uuid.New().String(), "-", "")
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
		defer func() {
			fatal := recover()
			if fatal != nil {
				stack := make([]string, 0, 20)
				for i := 0; i < 20; i++ {
					_, file, line, ok := runtime.Caller(i)
					if !ok {
						break
					}
					stack = append(stack, file+":"+strconv.Itoa(line))
				}
				logger.Logger.WithContext(ctx).Error(fatal, "\n", strings.Join(stack, "\n"))
				err = status.Errorf(codes.Internal, "panic with %v\n", fatal)
			}
		}()
		resp, err = handler(ctx, req)
		if err != nil {
			logger.Logger.WithContext(ctx).Error(err)
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
			return rpc.GetHeaders(ctx).Get(rpc.PrefixForSw + headerKey), nil
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
