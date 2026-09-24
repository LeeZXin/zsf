// Package client 提供基于 gRPC 的发布/订阅客户端，与 service/pubsub/server 配套使用。
// payload 为原始字节并原样透传，不做任何编解码，序列化方式由调用方自行决定。
package client

import (
	"context"
	"io"
	"time"

	"github.com/LeeZXin/zsf/container/hashmap"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/pubsub/grpc/proto"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	clientMap = hashmap.NewStringSyncMap[*Client]()
)

func init() {
	start.AddInit(func() {
		quit.AddHighPriorityShutdownHook(func() {
			clientMap.Range(func(_ string, client *Client) bool {
				client.conn.Close()
				return true
			})
		})
	}, -7)
}

// Client 是发布/订阅服务的 gRPC 客户端。
type Client struct {
	cc   proto.PubSubClient
	conn *grpc.ClientConn
}

// New 创建发布/订阅 gRPC 客户端。
// addr 为远端服务名或 host:port 地址，解析规则与 grpc client.Dial 一致。
func New(addr string) *Client {
	client, ok := clientMap.Load(addr)
	if ok {
		return client
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Logger.Fatal().Err(err).Msgf("grpc client init failed: %s", addr)
	}
	client = &Client{
		cc:   proto.NewPubSubClient(conn),
		conn: conn,
	}
	client, ok = clientMap.LoadOrStore(addr, client)
	if ok {
		conn.Close()
	}
	return client
}

// Publish 向远端发布消息，payload 为原始字节原样透传；调用失败仅记录日志。
func (c *Client) Publish(ctx context.Context, topic string, payload []byte) {
	_, err := c.cc.Publish(ctx, &proto.PublishRequest{
		Topic:   topic,
		AppName: instance.ApplicationName,
		Payload: payload,
	})
	if err != nil {
		logger.Logger.Error().Err(err).Msgf("pubsub client publish topic %s err: %v", topic, err)
	}
}

// Subscribe 订阅远端 topic 并阻塞接收推送，收到的消息（[]byte）依次调用回调，
// 直到流正常关闭（返回 nil）、异常断开（返回错误）或进程开始退出。
func (c *Client) Subscribe(topic string, callbacks ...func([]byte)) error {
	logger.Logger.Info().Msgf("trying to pubsub client subscribe topic: %s", topic)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-quit.Stopping():
			cancel()
		case <-ctx.Done():
		}
	}()
	stream, err := c.cc.Subscribe(ctx, &proto.SubscribeRequest{
		AppName: instance.ApplicationName,
		Topic:   topic,
	})
	if err != nil {
		return err
	}
	logger.Logger.Info().Msgf("successfully pubsub client subscribe topic: %s", topic)
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				return err
			}
			return nil
		}
		for _, callback := range callbacks {
			callback(resp.Payload)
		}
	}
}

// SubscribeForever 持续订阅远端 topic：Subscribe 返回（流断开）后每 5 秒重连一次；
// 进程退出（quit.Stopping）后停止重连。
func (c *Client) SubscribeForever(topic string, callbacks ...func([]byte)) {
	for {
		select {
		case <-quit.Stopping():
			return
		default:
		}
		err := c.Subscribe(topic, callbacks...)
		if err != nil {
			logger.Logger.Error().Err(err).Msgf("failed to pubsub client subscribe topic %s", topic)
		}
		select {
		case <-quit.Stopping():
			return
		case <-time.After(5 * time.Second):
		}
	}
}
