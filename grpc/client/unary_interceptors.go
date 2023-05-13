package grpcclient

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/skywalking"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"time"
)

// headerClientUnaryInterceptor 头部信息传递
func headerClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		//传递请求header
		md := rpc.GetHeaders(ctx)
		for k, v := range md {
			ctx = metadata.AppendToOutgoingContext(ctx, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, rpc.Source, appinfo.ApplicationName)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// promClientUnaryInterceptor prometheus监控
func promClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		begin := time.Now()
		defer func() {
			// prometheus监控
			prom.GrpcClientUnaryRequestTotal.WithLabelValues(method).Observe(float64(time.Since(begin).Milliseconds()))
		}()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// skywalkingUnaryInterceptor 接入skywalking
func skywalkingUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if skywalking.Tracer == nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		operationName := fmt.Sprintf("GRPC %s", method)
		span, err := skywalking.Tracer.CreateExitSpan(ctx, operationName, cc.Target(), func(key, value string) error {
			ctx = metadata.AppendToOutgoingContext(ctx, rpc.PrefixForSw+key, value)
			return nil
		})
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOGrpcUnaryClient)
		span.Tag(skywalking.TagGrpcMethod, method)
		span.Tag(skywalking.TagRpcScheme, skywalking.TagGrpcScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		err = invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.Error(time.Now(), err.Error())
		}
		return err
	}
}
