// Package server 提供发布/订阅服务的 gRPC 实现，
// 底层复用 zsf/pubsub 的包级进程内队列，将远端消息转投给本进程订阅者。
package server

import (
	"context"
	"io"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/pubsub/grpc/proto"
	"github.com/LeeZXin/zsf/pubsub/pubsub"
	"github.com/LeeZXin/zsf/quit"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// queue 包级进程内队列，容量 1024，服务内所有订阅者共享。
var (
	queue = pubsub.New(1024)
)

// server PubSub 服务的 gRPC 实现，请求经包级 queue 转发。
type server struct {
	proto.UnimplementedPubSubServer
}

// GrpcService 将 PubSub 服务注册到 grpc.Server。
func GrpcService(serv *grpc.Server) {
	proto.RegisterPubSubServer(serv, new(server))
}

// Publish 将 gRPC 发布请求转投给包级队列，由队列分发给对应 topic 的所有订阅者。
//
// 注意：队列以单次 Publish 的全部消息（[]any）为单位投递给回调，本服务每次
// 发布只有一个 []byte payload，Subscribe 的回调按 []any 断言后逐条推送。
func (s *server) Publish(_ context.Context, r *proto.PublishRequest) (*emptypb.Empty, error) {
	queue.Publish(r.GetTopic(), r.GetPayload())
	return new(emptypb.Empty), nil
}

// Subscribe 处理客户端订阅请求：在包级队列上注册订阅，回调通过流将消息推给客户端。
//
// 有两条退出路径：stream.Send 失败（errCh 收到错误）或客户端断开（ctx.Done），
// 退出时都会先退订。errCh 带缓冲且回调为非阻塞发送，保证 handler 返回后
// 排空期内的回调不会被阻塞；不要为 errCh 加 close，否则排空期回调会向已关闭的
// channel 发送导致 panic。
func (s *server) Subscribe(r *proto.SubscribeRequest, stream proto.PubSub_SubscribeServer) error {
	if r.AppName == "" || r.Topic == "" {
		return status.Error(codes.InvalidArgument, "app name or topic is empty")
	}
	errCh := make(chan error, 1)
	logger.Ctx(stream.Context()).Info().Msgf("app: %s subscribe: %s", r.AppName, r.Topic)
	subscriber := queue.Subscribe(r.Topic, func(payload any) {
		for _, msg := range payload.([]any) {
			err := stream.Send(&proto.SubscribeResponse{
				Payload: msg.([]byte),
			})
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}
	})
	defer queue.Unsubscribe(subscriber)
	select {
	case err := <-errCh:
		if err == io.EOF {
			logger.Ctx(stream.Context()).Info().Msgf("unsubscribe: %s - stream closed, no more input is available", r.GetTopic())
			return nil
		}
		logger.Ctx(stream.Context()).Error().Err(err).Msgf("unsubscribe: %s", r.GetTopic())
		return err
	case <-stream.Context().Done():
	case <-quit.Stopping():
	}
	return nil
}

// Queue 返回本服务持有的包级进程内队列，供同进程内的本地发布/订阅使用；
// 与远端 gRPC 订阅共用同一队列（发布到同一 topic 的消息本进程订阅者
// 也会收到）。
func Queue() pubsub.Queue {
	return queue
}
