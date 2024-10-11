package logger

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf-utils/executor"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/listutil"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf-utils/taskutil"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/nsqio/go-nsq"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	LogVersion = "v1.0.0"
)

type LogContent struct {
	Timestamp int64  `json:"timestamp"`
	Version   string `json:"version"`
	Level     string `json:"level"`
	Env       string `json:"env"`
	Region    string `json:"region"`
	Zone      string `json:"zone"`

	SourceIp   string `json:"sourceIp"`
	SourceType string `json:"sourceType"`

	AppId      string `json:"appId"`
	Content    string `json:"content"`
	TraceId    string `json:"traceId"`
	InstanceId string `json:"instanceId"`
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
		formatter: defaultFormatter,
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
	t, _ := time.Now().MarshalBinary()
	v := newLogContent(string(content), "kafka", entry)
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
	chunkExecuteFunc, _, chunkStopFunc, _ := taskutil.RunChunkTask[[]byte](10e6, func(content []taskutil.Chunk[[]byte]) {
		if content == nil || len(content) == 0 {
			return
		}
		send := make([][]byte, 0, len(content))
		for _, t := range content {
			send = append(send, t.Data)
		}
		_ = producer.MultiPublish(topic, send)
	}, 3*time.Second)
	quit.AddShutdownHook(func() {
		chunkStopFunc()
		producer.Stop()
	})
	ret := &nsqHook{
		topic:            topic,
		chunkExecuteFunc: chunkExecuteFunc,
		formatter:        defaultFormatter,
	}
	return ret
}

type nsqHook struct {
	topic            string
	formatter        logrus.Formatter
	chunkExecuteFunc taskutil.ChunkTaskExecuteFunc[[]byte]
}

func (*nsqHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (k *nsqHook) Fire(entry *logrus.Entry) error {
	content, err := k.formatter.Format(entry)
	if err != nil {
		return err
	}
	v := newLogContent(string(content), "nsq", entry)
	value, _ := json.Marshal(v)
	k.chunkExecuteFunc(value, len(value))
	return nil
}

type nsqLogger struct {
}

func (l *nsqLogger) Output(int, string) error {
	return nil
}

type lokiHook struct {
	pushUrl          string
	httpClient       *http.Client
	formatter        logrus.Formatter
	chunkExecuteFunc taskutil.ChunkTaskExecuteFunc[LogContent]
	flusher          *executor.Executor
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiHttpRequest struct {
	Streams []lokiStream `json:"streams"`
}

func newLokiHook() logrus.Hook {
	pushUrl := static.GetString("logger.loki.pushUrl")
	if pushUrl == "" {
		panic("empty logger.loki.pushUrl")
	}
	orgId := static.GetString("logger.loki.orgId")
	poolSize := static.GetInt("logger.loki.poolSize")
	if poolSize <= 0 {
		poolSize = 3
	}
	queueSize := static.GetInt("logger.loki.queueSize")
	if queueSize <= 0 {
		queueSize = 1024
	}
	flusher, _ := executor.NewExecutor(poolSize, queueSize, time.Minute, executor.CallerRunsStrategy)
	h := &lokiHook{
		pushUrl:    pushUrl,
		formatter:  &lokiLogFormatter{},
		httpClient: httputil.NewHttpClient(),
		flusher:    flusher,
	}
	chunkExecuteFunc, chunkFlushFunc, chunkStopFunc, _ := taskutil.RunChunkTask[LogContent](1024, func(logList []taskutil.Chunk[LogContent]) {
		h.flusher.Execute(func() {
			chunk := listutil.MapNe(logList, func(t taskutil.Chunk[LogContent]) LogContent {
				return t.Data
			})
			stream := h.convert2Stream(chunk)
			ctx, cancelFunc := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancelFunc()
			httputil.Post(ctx, h.httpClient, h.pushUrl, map[string]string{
				"X-Scope-OrgID": orgId,
			}, lokiHttpRequest{
				Streams: []lokiStream{
					stream,
				},
			}, nil)
		})
	}, 3*time.Second)
	quit.AddShutdownHook(func() {
		chunkFlushFunc()
		chunkStopFunc()
	}, true)
	h.chunkExecuteFunc = chunkExecuteFunc
	return h
}

func (*lokiHook) convert2Stream(logs []LogContent) lokiStream {
	stream := map[string]string{
		"env":        logs[0].Env,
		"region":     logs[0].Region,
		"zone":       logs[0].Zone,
		"sourceIp":   logs[0].SourceIp,
		"appId":      logs[0].AppId,
		"instanceId": logs[0].InstanceId,
	}
	values := listutil.MapNe(logs, func(t LogContent) []string {
		return []string{
			strconv.FormatInt(time.UnixMilli(t.Timestamp).UnixNano(), 10),
			t.Content,
		}
	})
	return lokiStream{
		Stream: stream,
		Values: values,
	}
}

func (*lokiHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (k *lokiHook) Fire(entry *logrus.Entry) error {
	content, err := k.formatter.Format(entry)
	if err != nil {
		return err
	}
	k.chunkExecuteFunc(newLogContent(string(content), "loki", entry), 1)
	return nil
}

func newLogContent(content, sourceType string, entry *logrus.Entry) LogContent {
	return LogContent{
		Timestamp:  entry.Time.UnixMilli(),
		Version:    LogVersion,
		Level:      entry.Level.String(),
		Env:        env.GetEnv(),
		Region:     common.GetRegion(),
		Zone:       common.GetZone(),
		SourceIp:   common.GetLocalIP(),
		SourceType: sourceType,
		AppId:      common.GetApplicationName(),
		Content:    content,
		TraceId:    GetTraceId(entry.Context),
		InstanceId: common.GetInstanceId(),
	}
}
