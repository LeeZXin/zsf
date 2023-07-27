package logger

import (
	"encoding/json"
	"github.com/IBM/sarama"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/sirupsen/logrus"
	"io"
	"strings"
	"time"
)

type kafkaWriter struct {
	topic     string
	producer  sarama.AsyncProducer
	errLogger *logrus.Logger
}

func (w *kafkaWriter) Write(p []byte) (int, error) {
	now := time.Now()
	t, _ := now.MarshalBinary()
	v := kafkaLogContent{
		Ip:              common.GetLocalIp(),
		InstanceId:      "xxx",
		ApplicationName: common.GetApplicationName(),
		LogContent:      string(p),
		SendTime:        now.UnixMilli(),
	}
	value, _ := json.Marshal(v)
	w.producer.Input() <- &sarama.ProducerMessage{
		Key:   sarama.ByteEncoder(t),
		Topic: w.topic,
		Value: sarama.ByteEncoder(value),
	}
	return len(p), nil
}

func (w *kafkaWriter) logErr() {
	go func() {
		for msg := range w.producer.Errors() {
			w.errLogger.Error(msg.Error())
		}
	}()
}

func newKafkaWriter(writer ...io.Writer) io.Writer {
	errLogger := newKafkaErrLogger(writer...)
	kafkaHosts := property.GetString("logger.kafka.hosts")
	if kafkaHosts == "" {
		errLogger.Panic("logger.kafka.hosts is empty")
	}
	topic := property.GetString("logger.kafka.topic")
	if topic == "" {
		errLogger.Panic("logger.kafka.topic is empty")
	}
	hosts := strings.Split(kafkaHosts, ",")
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForLocal       // Only wait for the leader to ack
	kafkaConfig.Producer.Compression = sarama.CompressionSnappy   // Compress messages
	kafkaConfig.Producer.Flush.Frequency = 500 * time.Millisecond // Flush batches every 500ms
	kafkaConfig.Net.DialTimeout = 5 * time.Second
	kafkaConfig.Net.ReadTimeout = 5 * time.Second
	kafkaConfig.Net.WriteTimeout = 5 * time.Second
	kafkaConfig.Net.SASL.Enable = property.GetBool("logger.kafka.sasl")
	kafkaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	kafkaConfig.Net.SASL.User = property.GetString("logger.kafka.username")
	kafkaConfig.Net.SASL.Password = property.GetString("logger.kafka.password")
	kafkaClient, err := sarama.NewClient(hosts, kafkaConfig)
	if err != nil {
		errLogger.Panic(err.Error())
	}
	producer, err := sarama.NewAsyncProducerFromClient(kafkaClient)
	if err != nil {
		errLogger.Panic(err.Error())
	}
	quit.AddShutdownHook(func() {
		_ = kafkaClient.Close()
		_ = producer.Close()
	})
	ret := &kafkaWriter{
		topic:     topic,
		producer:  producer,
		errLogger: errLogger,
	}
	ret.logErr()
	return ret
}

func newKafkaErrLogger(writer ...io.Writer) *logrus.Logger {
	logger := logrus.New()
	logger.SetReportCaller(true)
	logger.SetFormatter(&logFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	logger.SetOutput(io.MultiWriter(writer...))
	return logger
}

type kafkaLogContent struct {
	Ip              string `json:"ip"`
	InstanceId      string `json:"instanceId"`
	ApplicationName string `json:"applicationName"`
	LogContent      string `json:"logContent"`
	SendTime        int64  `json:"sendTime"`
}
