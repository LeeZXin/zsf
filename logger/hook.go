package logger

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/quit"
	"github.com/nsqio/go-nsq"
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
	kafkaHosts := static.GetString("logger.kafka.hosts")
	if kafkaHosts == "" {
		panic("logger.kafka.hosts is empty")
	}
	topic := static.GetString("logger.kafka.topic")
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
	if static.GetBool("logger.kafka.sasl") {
		mechanism := plain.Mechanism{
			Username: static.GetString("logger.kafka.username"),
			Password: static.GetString("logger.kafka.password"),
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
		SourceIp:    common.GetLocalIP(),
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

func newNsqHook() logrus.Hook {
	host := static.GetString("logger.nsq.host")
	if host == "" {
		panic("empty nsq host")
	}
	topic := static.GetString("logger.nsq.topic")
	if topic == "" {
		panic("empty nsq topic")
	}
	cnf := nsq.NewConfig()
	cnf.AuthSecret = static.GetString("logger.nsq.authSecret")
	producer, err := nsq.NewProducer(host, cnf)
	producer.SetLogger(&nsqLogger{}, nsq.LogLevelInfo)
	if err != nil {
		panic(err)
	}
	task, _ := taskutil.NewChunkTask[[]byte](10e6, func(content []taskutil.Chunk[[]byte]) {
		if content == nil || len(content) == 0 {
			return
		}
		send := make([][]byte, 0, len(content))
		for _, t := range content {
			send = append(send, t.Data)
		}
		_ = producer.MultiPublish(topic, send)
	}, 3*time.Second)
	task.Start()
	quit.AddShutdownHook(func() {
		task.Stop()
		producer.Stop()
	})
	ret := &nsqHook{
		topic:     topic,
		task:      task,
		formatter: &logFormatter{},
	}
	return ret
}

type nsqHook struct {
	topic     string
	formatter logrus.Formatter
	task      *taskutil.ChunkTask[[]byte]
}

func (*nsqHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (k *nsqHook) Fire(entry *logrus.Entry) error {
	content, err := k.formatter.Format(entry)
	if err != nil {
		return err
	}
	now := time.Now()
	v := LogContent{
		Timestamp:   now.UnixMilli(),
		Version:     LogVersion,
		Level:       entry.Level.String(),
		Env:         cmd.GetEnv(),
		SourceIp:    common.GetLocalIP(),
		Type:        "nsq",
		Application: common.GetApplicationName(),
		Content:     string(content),
		Tags:        []string{},
	}
	value, _ := json.Marshal(v)
	k.task.Execute(value, len(value))
	return nil
}

type nsqLogger struct {
}

func (l *nsqLogger) Output(int, string) error {
	return nil
}
