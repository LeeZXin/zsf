package grpcserver

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/skywalking"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"runtime"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strconv"
	"strings"
	"time"
)

// grpc.ServerStream 没有提供更换ctx的接口

type WrappedServerStream struct {
	grpc.ServerStream
	WrappedContext context.Context
}

func (w *WrappedServerStream) Context() context.Context {
	return w.WrappedContext
}

func WrapServerStream(stream grpc.ServerStream) *WrappedServerStream {
	if existing, ok := stream.(*WrappedServerStream); ok {
		return existing
	}
	return &WrappedServerStream{ServerStream: stream, WrappedContext: stream.Context()}
}

func headerStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		clone := copyIncomingContext(ctx)
		ctx = rpc.SetHeaders(ctx, clone)
		ctx = logger.AppendToMDC(ctx, clone)
		wrapped := WrapServerStream(ss)
		wrapped.WrappedContext = ctx
		return handler(srv, wrapped)
	}
}

// prometheusUnaryInterceptor 监控
func prometheusStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		begin := time.Now()
		err := handler(srv, ss)
		prom.GrpcServerStreamRequestTotal.
			WithLabelValues(info.FullMethod).
			Observe(float64(time.Since(begin).Milliseconds()))
		return err
	}
}

func logErrorStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
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
				logger.Logger.WithContext(ss.Context()).Error(fatal, "\n", strings.Join(stack, "\n"))
				err = status.Errorf(codes.Internal, "panic with %v\n", fatal)
			}
		}()
		err = handler(srv, ss)
		if err != nil {
			logger.Logger.WithContext(ss.Context()).Error(err)
		}
		return
	}
}

func skywalkingStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		if skywalking.Tracer == nil {
			return handler(srv, ss)
		}
		ctx := ss.Context()
		operationName := fmt.Sprintf("grpc %s", info.FullMethod)
		span, ctx, err := skywalking.Tracer.CreateEntrySpan(ctx, operationName, func(headerKey string) (string, error) {
			return rpc.GetHeaders(ctx).Get(rpc.PrefixForSw + headerKey), nil
		})
		if err != nil {
			return handler(srv, ss)
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOGrpcStreamServer)
		span.Tag(skywalking.TagGrpcMethod, info.FullMethod)
		span.Tag(skywalking.TagRpcScheme, skywalking.TagGrpcScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		wrapped := WrapServerStream(ss)
		wrapped.WrappedContext = ctx
		err = handler(srv, wrapped)
		if err != nil {
			span.Error(time.Now(), err.Error())
		}
		return
	}
}
