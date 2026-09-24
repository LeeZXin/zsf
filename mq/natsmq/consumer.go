/*
Package natsmq 提供基于 NATS JetStream pull consumer 的通用消费封装。

关于并发消费：

  - JetStream SDK 本身不能指定消费协程数量。Consume 回调是串行执行的：
    消息到达后由 SDK 依次同步调用 handler，一条处理完才会处理下一条；
  - 本封装使用 Messages() 迭代器 + N 个 worker goroutine 并发调用 Next()
    实现并发消费，并发数由 ConsumerConfig.Concurrency 指定（SDK 官方支持该用法，
    仅 ordered consumer 不允许并发）；
  - 每个 Messages() 内部只有一个 pullMessages goroutine 负责向 server 补拉消息
    （缓冲低于阈值时自动续拉），真正的处理并发来自业务侧 worker。

设计要点：

  - 一个 Consumer 对应一个 durable consumer，名称固定为 instance.ApplicationName，
    同名多实例共享消费进度，天然支持竞争消费模型（每条消息只被一个实例处理）；
  - 消息确认由封装统一处理：handler 返回 nil 自动 Ack，返回 error（含 panic）
    自动 Nak，配合 AckWait + MaxDeliver 由 server 重投递，实现至少一次语义；
  - Run 阻塞直到 ctx 取消后执行优雅停止：Drain 已缓冲消息 → 等待 worker 退出
    → 关闭底层连接；Run 返回后 Consumer 不可复用。

坑（务必注意）：

  - handler 由多个 worker goroutine 并发调用，需自行保证线程安全；
  - handler 返回 error 后消息被 Nak 立即重投：瞬间失败会形成热循环
    （消息反复进出 worker），建议设置有限的 MaxDeliver；达到 MaxDeliver 后
    仍失败的消息会被 Term 移出 consumer，如需死信需在 Term 前自行转发；
  - server 故障恢复：客户端固定无限重连（间隔 2s），server 恢复后自动续拉，
    未 Ack 的消息由 server 按 AckWait 重投，消费自动恢复、进程无需重启；
    仅致命错误（如认证失败）或外部调用 Close 会导致 Run 返回错误退出；
  - 多实例共享同一 durable consumer：各实例的 ConsumerConfig（AckWait / MaxDeliver /
    FilterSubjects）必须保持一致。CreateOrUpdateConsumer 是最后写入者赢，
    后启动的实例会覆盖已有配置，且 FilterSubjects / DeliverPolicy 变更
    会重置 server 端投递进度（按新策略重新开始）；
  - 消息处理不是幂等的场景下，AckWait 内进程崩溃会导致消息被重复投递，
    业务侧需自行去重（至少一次语义）。
*/
package natsmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/utils/threadutil"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

/*
ConsumerConfig 通用消费者配置。除 Addr 和 Stream 必填外，零值字段均使用默认值。

以下策略已固定，无需也不提供配置：
  - durable consumer 名固定为 instance.ApplicationName，同名多实例共享
    消费进度（竞争消费，每条消息只被一个实例处理）；
  - 投递策略固定 jetstream.DeliverAllPolicy：从 stream 起点投递（含创建
    前的存量消息）。若只要创建之后的新消息，改为 DeliverNewPolicy；
  - ack 策略固定 jetstream.AckExplicitPolicy（SDK 默认值）；
  - 重连策略固定无限重连（间隔 2s）：server 宕机后不断尝试重连，恢复后
    自动续拉消息，消费自动恢复，进程无需重启。
*/
type ConsumerConfig struct {
	/*
		Addr NATS server 地址，如 nats://127.0.0.1:4222，必填。
	*/
	Addr string
	/*
		Stream 消费的 stream 名称，必填。
	*/
	Stream string
	/*
		Concurrency 并发消费协程数，默认 1。
	*/
	Concurrency int
	/*
		AckWait 单条消息 ack 等待时长，默认 30s。
		handler 处理超时或进程崩溃导致未 ack 的消息，server 会在此时间后重新投递。
		处理耗时超过该值的业务必须调大，否则处理中消息会被重复投递。
	*/
	AckWait time.Duration
	/*
		MaxDeliver 消息最大投递次数，默认 -1（无限重投）。
		handler 返回 error 时消息立即重投（Nak），直到达到 MaxDeliver；
		最后一次投递仍失败的消息会被 Term 移出 consumer（不再重试，也不滞留 pending）。
	*/
	MaxDeliver int
	/*
		FilterSubjects 过滤消费的 subject 列表，为空表示消费 stream 下全部 subject。
	*/
	FilterSubjects []string
}

/*
Handler 消息处理函数。返回 nil 表示处理成功（自动 Ack），
返回 error 表示处理失败（自动 Nak，server 按 AckWait 重投递）。

注意：handler 由多个 worker goroutine 并发调用，需自行保证线程安全；
ctx 为 Run 传入的 ctx，进程退出时会被取消，handler 内的长任务应响应 ctx 取消。
*/
type Handler func(ctx context.Context, msg jetstream.Msg) error

/*
Consumer 通用 JetStream 消费者，持有底层连接与 durable consumer。
*/
type Consumer struct {
	conn     *nats.Conn
	js       jetstream.JetStream
	consumer jetstream.Consumer
	cfg      ConsumerConfig
}

// defaultCreateTimeout 创建 consumer 的默认超时时间。
const defaultCreateTimeout = 3 * time.Second

/*
NewConsumer 连接 NATS 并创建/更新 durable consumer。失败时自动关闭连接并返回错误。
*/
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	if cfg.Addr == "" || cfg.Stream == "" {
		return nil, errors.New("natsmq: Addr and Stream are required")
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = 30 * time.Second
	}
	if cfg.MaxDeliver == 0 {
		cfg.MaxDeliver = -1
	}
	// 无限重连（间隔 2s，nats 默认）：server 宕机后自动恢复，见 Config 注释
	conn, err := nats.Connect(cfg.Addr, nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), defaultCreateTimeout)
	defer cancelFunc()
	jc, err := js.CreateOrUpdateConsumer(ctx, cfg.Stream, jetstream.ConsumerConfig{
		// Name/DeliverPolicy/AckPolicy 为框架固定策略，见 Config 注释。
		// 只设 Name 即为 durable consumer；Durable 与 Name 是同一标识的别名，
		// 两者必须相等。切勿设成 instance.ID（每次启动都变的随机 UUID），
		// 否则每次重启都会新建一个永存不清理的孤儿 durable
		Name:           instance.ApplicationName,
		Durable:        instance.ApplicationName,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckWait:        cfg.AckWait,
		MaxDeliver:     cfg.MaxDeliver,
		FilterSubjects: cfg.FilterSubjects,
		Metadata: map[string]string{
			"ip": env.LocalIP,
		},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Consumer{
		conn:     conn,
		js:       js,
		consumer: jc,
		cfg:      cfg,
	}, nil
}

/*
Run 启动消费并阻塞，直到 ctx 取消后完成优雅停止返回。

执行流程：
 1. 建立 Messages 迭代器，启动 Concurrency 个 worker goroutine 并发处理消息；
 2. ctx 取消后调用 Drain：已缓冲的消息会被 worker 处理完毕；
    处理中的消息不受影响，由其 handler 自行响应 ctx 取消；
 3. 等待全部 worker 退出后关闭连接。

正常停止（ctx 取消或迭代器被 Close）返回 nil，worker 异常退出返回错误。
Run 返回后 Consumer 不可复用。
*/
func (c *Consumer) Run(ctx context.Context, handler Handler) error {
	if handler == nil {
		return errors.New("natsmq: handler is required")
	}
	// 不传 Pull 选项，使用 SDK 默认值（缓冲 500 条、单次拉取等待 30s）
	cc, err := c.consumer.Messages()
	if err != nil {
		return err
	}
	defer c.Close()

	logger.Logger.Info().Msgf("starting consumer stream %s concurrency %d", c.cfg.Stream, c.cfg.Concurrency)

	var wg sync.WaitGroup
	workerErr := make(chan error, c.cfg.Concurrency)
	for i := 0; i < c.cfg.Concurrency; i++ {
		wg.Go(func() {
			if err := c.consumeLoop(ctx, cc, handler); err != nil {
				workerErr <- err
			}
		})
	}

	var runErr error
	select {
	case <-ctx.Done():
		// 优雅停止：Drain 后已缓冲消息仍可被 Next 取出，
		// 缓冲耗尽后 Next 返回 ErrMsgIteratorClosed，worker 自然退出
		cc.Drain()
	case runErr = <-workerErr:
		// 某个 worker 异常退出（如连接断开），停止迭代器让其余 worker 退出
		cc.Stop()
	}
	wg.Wait()
	logger.Logger.Info().Msgf("natsmq consumer stopped stream %s", c.cfg.Stream)
	return runErr
}

/*
consumeLoop worker 循环：从迭代器取消息并交给 handler。
迭代器因 Drain/Stop 关闭时正常返回 nil；因连接彻底关闭（认证失败等致命
错误 nats 不重试、或外部调用 Close）而关闭时返回错误，触发 Run 退出
fail-fast，避免 worker 静默退出后 Run 假死（server 宕机不在此列，
无限重连会兜住，worker 一直阻塞在 Next 直到重连成功）；
心跳超时（server 短暂无响应）是瞬态错误，迭代器仍可用，继续循环即可。
*/
func (c *Consumer) consumeLoop(ctx context.Context, cc jetstream.MessagesContext, handler Handler) error {
	for {
		msg, err := cc.Next()
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				if errors.Is(err, nats.ErrConnectionClosed) {
					return fmt.Errorf("natsmq connection closed: %w", err)
				}
				return nil
			}
			if errors.Is(err, jetstream.ErrNoHeartbeat) {
				// server 在心跳窗口内没有响应（卡顿/网络抖动）：
				// SDK 已重置 pending，下次 Next 会自动发起新的 pull 请求
				logger.Logger.Warn().
					Err(err).
					Str("stream", c.cfg.Stream).
					Msg("natsmq heartbeat timeout, continuing")
				continue
			}
			return fmt.Errorf("natsmq consume iterator: %w", err)
		}
		c.handleMsg(ctx, msg, handler)
	}
}

/*
handleMsg 执行 handler 并按结果确认消息：成功 Ack，失败（含 panic）Nak 立即重投。
已达 MaxDeliver 上限时 Nak 无效（server 不再重投），此时改为 Term 移出 consumer，
避免消息永远滞留在 pending 状态。
*/
func (c *Consumer) handleMsg(ctx context.Context, msg jetstream.Msg, handler Handler) {
	var handleErr error
	if panicErr := threadutil.RunSafe(func() {
		handleErr = handler(ctx, msg)
	}); panicErr != nil {
		// handler panic 视为处理失败，Nak 重投
		handleErr = panicErr
	}

	subject := msg.Subject()
	var seq, numDelivered uint64
	if md, err := msg.Metadata(); err == nil {
		seq = md.Sequence.Stream
		numDelivered = md.NumDelivered
	}

	if handleErr == nil {
		if err := msg.Ack(); err != nil {
			logger.Logger.Error().
				Err(err).
				Str("subject", subject).
				Uint64("seq", seq).
				Msg("natsmq ack failed")
		}
		return
	}
	if c.cfg.MaxDeliver > 0 && numDelivered >= uint64(c.cfg.MaxDeliver) {
		// 最后一次投递仍失败：server 不会再重投，Term 移出 consumer。
		// 如需死信队列，可在这里将 msg.Data() 发布到死信 subject/stream 后再 Term
		logger.Logger.Error().
			Err(handleErr).
			Str("subject", subject).
			Uint64("seq", seq).
			Uint64("delivered", numDelivered).
			Msg("natsmq handler failed, max deliver reached, message terminated")
		if err := msg.Term(); err != nil {
			logger.Logger.Error().
				Err(err).
				Str("subject", subject).
				Uint64("seq", seq).
				Msg("natsmq term failed")
		}
		return
	}
	logger.Logger.Error().
		Err(handleErr).
		Str("subject", subject).
		Uint64("seq", seq).
		Uint64("delivered", numDelivered).
		Msg("natsmq handler failed, message will be redelivered")
	if err := msg.Nak(); err != nil {
		logger.Logger.Error().
			Err(err).
			Str("subject", subject).
			Uint64("seq", seq).
			Msg("natsmq nak failed")
	}
}

/*
Close 关闭底层连接，幂等可重复调用。Run 结束时自动调用；
仅创建 consumer 而不使用 Run 的场景需手动调用以释放连接。
*/
func (c *Consumer) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
