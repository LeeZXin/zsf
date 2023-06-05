package grpcclient

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/skywalking"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strings"
	"time"
)

func headerStreamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		//传递请求header
		md := rpc.GetHeaders(ctx)
		for k, v := range md {
			if !strings.HasPrefix(k, ":") {
				ctx = metadata.AppendToOutgoingContext(ctx, k, v)
			}
		}
		ctx = metadata.AppendToOutgoingContext(ctx, rpc.Source, common.GetApplicationName())
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func promStreamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		begin := time.Now()
		defer func() {
			// prometheus监控
			prom.GrpcClientUnaryRequestTotal.WithLabelValues(method).Observe(float64(time.Since(begin).Milliseconds()))
		}()
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func skywalkingStreamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if skywalking.Tracer == nil {
			return streamer(ctx, desc, cc, method, opts...)
		}
		operationName := fmt.Sprintf("GRPC %s", method)
		span, err := skywalking.Tracer.CreateExitSpan(ctx, operationName, cc.Target(), func(key, value string) error {
			ctx = metadata.AppendToOutgoingContext(ctx, rpc.PrefixForSw+key, value)
			return nil
		})
		if err != nil {
			return streamer(ctx, desc, cc, method, opts...)
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOGrpcStreamClient)
		span.Tag(skywalking.TagGrpcMethod, method)
		span.Tag(skywalking.TagRpcScheme, skywalking.TagGrpcScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.Error(time.Now(), err.Error())
		}
		return stream, err
	}
}
