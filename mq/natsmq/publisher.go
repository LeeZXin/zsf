package natsmq

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

/*
AutoCreateSubjectConfig 自动创建 stream 的配置，可选。
设置后 NewPublisher 会确保 stream 存在：不存在则按 Subjects / MaxAge 创建
（Retention 固定 LimitsPolicy，Storage 用默认 FileStorage），已存在则不做
任何修改，避免覆盖其他发布者或运维配置好的 stream。
Subjects 必须显式声明——subject 与 stream 名无关，不会用 stream 名隐式顶替。
*/
type AutoCreateSubjectConfig struct {
	/*
		Stream 要确保存在的 stream 名称，必填。
	*/
	Stream string
	/*
		Subjects stream 订阅的 subject 列表，必填。
		stream 已存在时本字段被忽略（不修改既有配置）。
	*/
	Subjects []string
	/*
		MaxAge 自动创建 stream 时消息保留时长，0 表示不限制。
		建议按业务审计/回放需求设置（如 7*24h），否则 stream 会无限增长。
		stream 已存在时本字段被忽略。
	*/
	MaxAge time.Duration
}

/*
PublisherConfig 通用发布者配置。除 Addr 必填外，零值字段均使用默认行为。
*/
type PublisherConfig struct {
	/*
		Addr NATS server 地址，如 nats://127.0.0.1:4222，必填。
	*/
	Addr string

	/*
		Stream 期望消息落入的 stream 名称，可选。
		设置后每次 Publish 自动附带 jetstream.WithExpectStream 校验：
		消息确实落入该 stream 才算发布成功；不负责创建 stream
		（自动创建见 AutoCreateSubjectConfig）。
		未设置时，subject 若无任何 stream 匹配，发布返回错误（SDK 默认
		重试 2 次，间隔 250ms），但多个 stream 匹配同一 subject 时消息
		可能落入非预期 stream。
	*/
	Stream string

	/*
		AutoCreateSubjectConfig 自动创建 stream 的配置，可选。
		设置后 NewPublisher 会确保其中声明的 stream 存在（不存在则创建），
		Subjects 必须显式声明，见 AutoCreateSubjectConfig 注释。
	*/
	AutoCreateSubjectConfig *AutoCreateSubjectConfig
}

/*
Publisher 通用 JetStream 发布者，持有底层连接。
连接策略与 Consumer 一致：固定无限重连，server 宕机期间发布请求
缓冲在客户端，恢复后自动 flush，无需进程重启。
*/
type Publisher struct {
	conn *nats.Conn
	js   jetstream.JetStream
	// opts 固定的发布选项，NewPublisher 时构建一次，Publish 并发复用
	//（SDK 的 opt 函数只写入每次调用独立的 pubOpts，不修改本 slice）
	opts []jetstream.PublishOpt
}

/*
NewPublisher 连接 NATS 并返回发布者。失败时自动关闭连接并返回错误。
cfg.AutoCreateSubjectConfig 非空时会先确保 stream 存在
（不存在则自动创建，已存在不修改）。

典型用法（配合框架 shutdown hook 释放连接）:

	p, err := natsmq.NewPublisher(natsmq.PublisherConfig{Addr: addr, Stream: stream})
	if err != nil { ... }
	quit.AddHighPriorityShutdownHook(p.Close)
*/
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if cfg.Addr == "" {
		return nil, errors.New("natsmq: Addr is required")
	}
	if cfg.AutoCreateSubjectConfig != nil {
		if cfg.AutoCreateSubjectConfig.Stream == "" {
			return nil, errors.New("natsmq: AutoCreateSubjectConfig.Stream is required")
		}
		if len(cfg.AutoCreateSubjectConfig.Subjects) == 0 {
			return nil, errors.New("natsmq: AutoCreateSubjectConfig.Subjects is required")
		}
	}
	// RetryOnFailedConnect(false)：启动时 server 不可达立即返回错误
	// （由调用方决定重试/重启），不做无限等待；连接建立后的断线重连仍无限
	conn, err := nats.Connect(cfg.Addr, nats.MaxReconnects(-1), nats.RetryOnFailedConnect(false))
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	// 自动创建 stream（仅当不存在）：已存在则保持原样，
	// 不覆盖其他发布者或运维配置好的 stream，见 AutoCreateSubjectConfig 注释
	if cfg.AutoCreateSubjectConfig != nil {
		createCtx, cancel := context.WithTimeout(context.Background(), defaultCreateTimeout)
		_, err = js.CreateStream(createCtx, jetstream.StreamConfig{
			Name:     cfg.AutoCreateSubjectConfig.Stream,
			Subjects: cfg.AutoCreateSubjectConfig.Subjects,
			// LimitsPolicy：消息消费后保留（可回放、可挂多个 consumer），
			// 按 MaxAge 滚动清理；Storage 零值即 FileStorage（默认）
			Retention: jetstream.LimitsPolicy,
			MaxAge:    cfg.AutoCreateSubjectConfig.MaxAge,
		})
		cancel()
		if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			conn.Close()
			return nil, err
		}
		// ErrStreamNameAlreadyInUse：stream 已存在，直接使用
	}
	// 固定发布选项只构建一次，见 Publisher.opts 注释
	opts := []jetstream.PublishOpt{
		jetstream.WithRetryAttempts(jetstream.DefaultPubRetryAttempts),
		jetstream.WithRetryWait(jetstream.DefaultPubRetryWait),
	}
	if cfg.Stream != "" {
		opts = append(opts, jetstream.WithExpectStream(cfg.Stream))
	}
	return &Publisher{
		conn: conn,
		js:   js,
		opts: opts,
	}, nil
}

/*
Publish 同步发布一条消息到指定 subject，等待 server 确认后返回。
返回 nil 表示消息已持久化到 stream。

固定行为（无需也不提供配置）：
  - subject 无 stream 匹配（NoResponders）时自动重试 2 次、间隔 250ms
    （jetstream.DefaultPubRetryAttempts / DefaultPubRetryWait，显式写死，
    不随 SDK 默认值变化）；
  - PublisherConfig.Stream 非空时自动附带 WithExpectStream 校验：
    消息必须落入指定 stream 才算发布成功；
  - ctx 未设置 deadline 时 SDK 自动附加 5s 超时；server 宕机期间请求
    缓冲在客户端等待重连，超过 ctx 超时则返回错误，由调用方决定重试。
*/
func (p *Publisher) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := p.js.Publish(ctx, subject, data, p.opts...)
	return err
}

/*
PublishMsg 同步发布带 header 的消息，固定行为与 Publish 一致。
*/
func (p *Publisher) PublishMsg(ctx context.Context, msg *nats.Msg) error {
	_, err := p.js.PublishMsg(ctx, msg, p.opts...)
	return err
}

/*
Close 关闭底层连接，幂等可重复调用。发布者无后台 goroutine，
不使用时调用一次即可（或注册 shutdown hook，见 NewPublisher）。
*/
func (p *Publisher) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
}

func (p *Publisher) Conn() *nats.Conn {
	return p.conn
}
