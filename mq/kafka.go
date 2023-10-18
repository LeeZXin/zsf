package mq

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf-utils/threadutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"sync"
	"time"
)

type KafkaConfig struct {
	Brokers                []string `json:"brokers"`
	Topic                  string   `json:"topic"`
	GroupId                string   `json:"groupId"`
	Offset                 int64    `json:"offset"`
	StartFromFirstOffset   bool     `json:"startFromFirstOffset"`
	StartAtTimestampOffset int64    `json:"startAtTimestampOffset"`
	Username               string   `json:"username"`
	Password               string   `json:"password"`
	SaslMechanism          string   `json:"saslMechanism"`
}

func (c *KafkaConfig) Validate() error {
	if c.Brokers == nil || len(c.Brokers) == 0 {
		return errors.New("empty broker config")
	}
	if c.Topic == "" {
		return errors.New("empty topic")
	}
	if c.GroupId == "" {
		return errors.New("empty groupId")
	}
	return nil
}

type KafkaConsumer struct {
	config     KafkaConfig
	reader     *kafka.Reader
	startOnce  sync.Once
	stopOnce   sync.Once
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func NewKafkaConsumer(config KafkaConfig) (*KafkaConsumer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	var dialer *kafka.Dialer
	if config.Username != "" && config.Password != "" && config.SaslMechanism != "" {
		var alg scram.Algorithm
		switch config.SaslMechanism {
		case "sha512":
			alg = scram.SHA512
			break
		case "sha256":
			alg = scram.SHA256
			break
		default:
			return nil, errors.New("scram.Algorithm error")
		}
		mechanism, err := scram.Mechanism(alg, config.Username, config.Password)
		if err != nil {
			return nil, err
		}
		dialer = &kafka.Dialer{
			Timeout:       10 * time.Second,
			DualStack:     true,
			SASLMechanism: mechanism,
		}
	} else {
		dialer = kafka.DefaultDialer
	}
	readerConfig := kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupId,
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		Dialer:         dialer,
	}
	reader := kafka.NewReader(readerConfig)
	if config.StartAtTimestampOffset > 0 {
		err := reader.SetOffsetAt(context.Background(), time.UnixMilli(config.StartAtTimestampOffset))
		if err != nil {
			return nil, err
		}
	} else if config.Offset > 0 {
		err := reader.SetOffset(config.Offset)
		if err != nil {
			return nil, err
		}
	} else if config.StartFromFirstOffset {
		readerConfig.StartOffset = kafka.FirstOffset
	} else {
		readerConfig.StartOffset = kafka.LastOffset
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &KafkaConsumer{
		config:     config,
		reader:     reader,
		startOnce:  sync.Once{},
		stopOnce:   sync.Once{},
		ctx:        ctx,
		cancelFunc: cancelFunc,
	}, nil
}

func (c *KafkaConsumer) Consume(consumer func(context.Context, int64, string) error, autoCommit bool, executorNum int) {
	if consumer == nil {
		return
	}
	c.startOnce.Do(func() {
		for i := 0; i < executorNum; i++ {
			go func() {
				logger.Logger.Infof("start consume topic: %s, autoCommit: %v, groupId: %s", c.config.Topic, autoCommit, c.config.GroupId)
				if autoCommit {
					c.consumeAutoCommit(consumer)
				} else {
					c.consumeNotAutoCommit(consumer)
				}
			}()
		}
	})
}

func (c *KafkaConsumer) consumeAutoCommit(consumer func(context.Context, int64, string) error) {
	ck := context.Background()
	for {
		if c.isDone() {
			return
		}
		m, err := c.reader.ReadMessage(ck)
		if err != nil {
			if c.isDone() {
				return
			}
			logger.Logger.Error("failed to read message:", err)
			time.Sleep(time.Second)
			continue
		}
		fatal := threadutil.RunSafe(func() {
			mdcCtx := logger.AppendToMDC(context.Background(), map[string]string{
				logger.TraceId: idutil.RandomUuid(),
			})
			_ = consumer(mdcCtx, m.Offset, string(m.Value))
		})
		if fatal != nil {
			logger.Logger.Error("failed to consume message:", err)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (c *KafkaConsumer) consumeNotAutoCommit(consumer func(context.Context, int64, string) error) {
	for {
		if c.isDone() {
			return
		}
		m, err := c.reader.FetchMessage(c.ctx)
		if err != nil {
			if c.isDone() {
				return
			}
			logger.Logger.Error("failed to read message:", err)
			time.Sleep(time.Second)
			continue
		}
		fatal := threadutil.RunSafe(func() {
			mdcCtx := logger.AppendToMDC(context.Background(), map[string]string{
				logger.TraceId: idutil.RandomUuid(),
			})
			err = consumer(mdcCtx, m.Offset, string(m.Value))
		})
		if fatal == nil && err == nil {
			if err2 := c.reader.CommitMessages(c.ctx, m); err2 != nil {
				logger.Logger.Error("failed to commit messages:", err)
				time.Sleep(100 * time.Millisecond)
			}
		} else if fatal != nil {
			logger.Logger.Error("failed to commit messages:", err)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (c *KafkaConsumer) Stop() {
	c.stopOnce.Do(func() {
		logger.Logger.Infof("stop consume topic: %s, groupId: %s", c.config.Topic, c.config.GroupId)
		c.cancelFunc()
		if err := c.reader.Close(); err != nil {
			logger.Logger.Error("failed to close reader:", err)
		}
	})
}

func (c *KafkaConsumer) isDone() bool {
	return c.ctx.Err() != nil
}
