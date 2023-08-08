package logger

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

const (
	LogVersion = "1"
)

type LogContent struct {
	Timestamp int64  `json:"@timestamp"`
	Version   string `json:"@version"`
	Level     string `json:"level"`
	Env       string `json:"env"`

	SourceIp string `json:"sourceIp"`
	Type     string `json:"type"`
	FileName string `json:"fileName"`

	Application string   `json:"application"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
}

func newKafkaHook() logrus.Hook {
	kafkaHosts := property.GetString("logger.kafka.hosts")
	if kafkaHosts == "" {
		panic("logger.kafka.hosts is empty")
	}
	topic := property.GetString("logger.kafka.topic")
	if topic == "" {
		panic("logger.kafka.topic is empty")
	}
	kw := &kafka.Writer{
		Addr:         kafka.TCP(strings.Split(kafkaHosts, ",")...),
		Topic:        topic,
		MaxAttempts:  1,
		BatchSize:    100,
		BatchTimeout: 3 * time.Second,
		Async:        true,
		Compression:  kafka.Snappy,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireNone,
	}
	if property.GetBool("logger.kafka.sasl") {
		mechanism := plain.Mechanism{
			Username: property.GetString("logger.kafka.username"),
			Password: property.GetString("logger.kafka.password"),
		}
		kw.Transport = &kafka.Transport{
			SASL: mechanism,
		}
	}
	quit.AddShutdownHook(func() {
		_ = kw.Close()
	})
	ret := &kafkaHook{
		writer:    kw,
		formatter: &logFormatter{},
	}
	return ret
}

type kafkaHook struct {
	formatter logrus.Formatter
	writer    *kafka.Writer
}

func (*kafkaHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (k *kafkaHook) Fire(entry *logrus.Entry) error {
	content, err := k.formatter.Format(entry)
	if err != nil {
		return err
	}
	now := time.Now()
	t, _ := now.MarshalBinary()
	v := LogContent{
		Timestamp:   now.UnixMilli(),
		Version:     LogVersion,
		Level:       entry.Level.String(),
		Env:         cmd.GetEnv(),
		SourceIp:    common.GetLocalIp(),
		Type:        "kafka",
		Application: common.GetApplicationName(),
		Content:     string(content),
		Tags:        []string{},
	}
	value, _ := json.Marshal(v)
	_ = k.writer.WriteMessages(context.Background(), kafka.Message{
		Key:   t,
		Value: value,
	})
	return nil
}
