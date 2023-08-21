package mq

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	"github.com/nsqio/go-nsq"
	"sync"
	"time"
)

type nsqLogger struct {
}

func (l *nsqLogger) Output(calldepth int, s string) error {
	logger.Logger.Info(s)
	return nil
}

type NsqConsumerConfig struct {
	Topic        string   `json:"topic"`
	Channel      string   `json:"channel"`
	Addrs        []string `json:"addrs"`
	ExecutorNums int      `json:"executorNums"`
	AuthSecret   string   `json:"authSecret"`
}

func (c *NsqConsumerConfig) Validate() error {
	if c.Topic == "" {
		return errors.New("empty topic")
	}
	if c.Channel == "" {
		return errors.New("empty channel")
	}
	if c.Addrs == nil || len(c.Addrs) == 0 {
		return errors.New("empty addrs")
	}
	if c.ExecutorNums <= 0 {
		return errors.New("wrong ExecutorNums")
	}
	return nil
}

type NsqConsumer struct {
	config    NsqConsumerConfig
	consumer  *nsq.Consumer
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewNsqConsumer(nsqConfig NsqConsumerConfig) (*NsqConsumer, error) {
	err := nsqConfig.Validate()
	if err != nil {
		return nil, err
	}
	config := nsq.NewConfig()
	config.AuthSecret = nsqConfig.AuthSecret
	consumer, err := nsq.NewConsumer(nsqConfig.Topic, nsqConfig.Channel, config)
	if err != nil {
		return nil, err
	}
	consumer.SetLogger(&nsqLogger{}, nsq.LogLevelInfo)
	return &NsqConsumer{
		config:   nsqConfig,
		consumer: consumer,
	}, nil
}

func (c *NsqConsumer) Stop() {
	c.stopOnce.Do(func() {
		c.consumer.Stop()
	})
}

func (c *NsqConsumer) ConsumeNsqds(consumer func(context.Context, *nsq.Message) error) {
	if consumer == nil {
		return
	}
	c.startOnce.Do(func() {
		go c.connect(consumer, 0)
	})
}

func (c *NsqConsumer) ConsumeLookupds(consumer func(context.Context, *nsq.Message) error) {
	if consumer == nil {
		return
	}
	c.startOnce.Do(func() {
		go c.connect(consumer, 1)
	})
}

func (c *NsqConsumer) connect(consumer func(context.Context, *nsq.Message) error, targetType int) {
	c.consumer.AddConcurrentHandlers(nsq.HandlerFunc(func(message *nsq.Message) error {
		mdcCtx := logger.AppendToMDC(context.Background(), map[string]string{
			logger.TraceId: idutil.RandomUuid(),
		})
		return consumer(mdcCtx, message)
	}), c.config.ExecutorNums)
	var err error
	if targetType == 0 {
		logger.Logger.Info("consume nsqds:", c.config)
		err = c.consumer.ConnectToNSQDs(c.config.Addrs)
		for err != nil {
			time.Sleep(time.Second)
			logger.Logger.Info("retry consume nsqds:", c.config)
			err = c.consumer.ConnectToNSQDs(c.config.Addrs)
		}
	} else {
		logger.Logger.Info("consume nsqlookupds:", c.config)
		err = c.consumer.ConnectToNSQLookupds(c.config.Addrs)
		for err != nil {
			time.Sleep(time.Second)
			logger.Logger.Info("retry consume nsqlookupds:", c.config)
			err = c.consumer.ConnectToNSQDs(c.config.Addrs)
		}
	}
}

type NsqProducerConfig struct {
	Addr       string `json:"addr"`
	AuthSecret string `json:"authSecret"`
}

func (c *NsqProducerConfig) Validate() error {
	if c.Addr == "" {
		return errors.New("empty addr")
	}
	return nil
}

func NewNsqProducer(config NsqProducerConfig) (*nsq.Producer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	cnf := nsq.NewConfig()
	cnf.AuthSecret = config.AuthSecret
	producer, err := nsq.NewProducer(config.Addr, cnf)
	if err != nil {
		return nil, err
	}
	producer.SetLogger(&nsqLogger{}, nsq.LogLevelInfo)
	return producer, nil
}
