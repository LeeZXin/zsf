package logger

import (
	"context"
	"io"
	"log"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/utils/chunktask"
	"github.com/LeeZXin/zsf/utils/jsonutil"

	"github.com/bytedance/sonic"
	"github.com/grafana/loki/pkg/push"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

/*
本文件实现 Loki 日志输出器：将 zerolog 日志异步批量推送到 Grafana Loki（gRPC push 接口）。

设计要点：
  - 实现 zerolog.LevelWriter 接口，作为 zerolog 的输出目标，并透传写回原输出（stdout/文件）
  - 仅 Info 及以上级别推送到 Loki，Debug 日志不发送
  - stream labels 仅保留 service_name 低基数维度；level、ip、traceId 以结构化元数据携带。
    其中 detected_level 由客户端写入：Loki 单机模式下 gRPC 推送直达 ingester、
    不经 distributor 的服务端级别检测，客户端写入才能让 Grafana 日志下钻识别级别
  - chunktask 批量聚合：积攒 100 条或每 10 秒推送一次，减少 RPC 次数
*/

/*
logEntry 对应 zerolog 输出的 JSON 日志行，WriteLevel 中解析后转换为 Loki 条目。
*/
type logEntry struct {
	Level   string `json:"level,omitempty"`
	Ip      string `json:"ip,omitempty"`
	Time    string `json:"time,omitempty"`
	Caller  string `json:"caller,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	TraceId string `json:"traceId,omitempty"`
}

/*
lokiEntry 单条日志在 Loki 中的正文（Entry.Line 字段），仅保留排查必需的字段。
*/
type lokiEntry struct {
	Caller  string `json:"caller,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

/*
lokiHook Loki 日志输出器。

字段：
  - Writer: 原始输出目标（stdout/文件），Write 直接透传
  - task:   批量聚合器，触发时把积攒的 streams 一次 push
  - client: Loki gRPC push 客户端
*/
type lokiHook struct {
	io.Writer
	task   *chunktask.Task[push.Stream]
	client push.PusherClient
	ctx    context.Context
}

/*
newLokiWriter 创建 Loki 输出器，包装原输出 out 并返回 zerolog.LevelWriter。

  - logger.loki.grpc：Loki gRPC push 地址，为空则 Fatal 退出（logger.loki.enable 开启时必须配置）
  - 连接注册低优先级 shutdown hook，进程退出前关闭
  - 批量策略：积攒 100 条立即推送，或每 10 秒推送一次
*/
func newLokiWriter(out io.Writer) zerolog.LevelWriter {
	endpoint := static.GetString("logger.loki.grpc")
	if endpoint == "" {
		log.Fatalln("logger.loki.grpc is empty")
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalln(err)
	}
	client := push.NewPusherClient(conn)
	w := &lokiHook{
		Writer: out,
		client: client,
		ctx:    context.Background(),
	}
	w.task = chunktask.New(100, 10*time.Second, func(streams []push.Stream) {
		_, err := w.client.Push(w.ctx, &push.PushRequest{
			Streams: streams,
			Format:  "loki",
		})
		if err != nil {
			log.Println(err)
		}
	})
	quit.AddFinalShutdownHook(func() {
		w.task.Sync()
		conn.Close()
	})
	return w
}

/*
WriteLevel 实现 zerolog.LevelWriter 接口：解析 JSON 日志行，
Info 及以上级别推送到 Loki（级别作为 detected_level 结构化元数据携带），
最后将原始字节透传给原输出。
*/
func (l *lokiHook) WriteLevel(level zerolog.Level, msg []byte) (int, error) {
	if level >= zerolog.InfoLevel {
		var e logEntry
		_ = sonic.Unmarshal(msg, &e)
		timestamp, err := time.ParseInLocation(zerolog.TimeFieldFormat, e.Time, time.Local)
		if err != nil {
			// 解析失败时回退当前时间，避免零值时间戳导致 Loki 拒收
			timestamp = time.Now()
		}
		traceId := e.TraceId
		if traceId == "" {
			traceId = "-"
		}
		// loki无panic关键词
		levelStr := level.String()
		if level == zerolog.PanicLevel {
			levelStr = "critical"
		}
		l.task.Add(push.Stream{
			Labels: convertLabels(),
			Entries: []push.Entry{
				{
					Timestamp: timestamp,
					Line: jsonutil.MarshalStringIgnoreErr(lokiEntry{
						Caller:  e.Caller,
						Error:   e.Error,
						Message: e.Message,
					}),
					StructuredMetadata: push.LabelsAdapter{
						{
							Name:  "traceId",
							Value: traceId,
						},
						{
							Name:  "detected_level",
							Value: levelStr,
						},
						{
							Name:  "ip",
							Value: e.Ip,
						},
					},
				},
			},
		})
		if level == zerolog.FatalLevel || level == zerolog.PanicLevel {
			l.task.Sync()
		}
	}
	return l.Write(msg)
}

/*
convertLabels 构建 Loki labels 字符串，仅包含 service_name 索引维度；
level、ip 等查询维度由 WriteLevel 写入结构化元数据。
*/
func convertLabels() string {
	sb := strings.Builder{}
	sb.WriteString("{")
	sb.WriteString("service_name=\"")
	sb.WriteString(instance.ApplicationName)
	sb.WriteString("\"}")
	return sb.String()
}
