/*
Package logger 提供基于 zerolog 的全局日志实例和 TraceId 上下文传递。

核心能力：
 1. 全局 Logger：start 钩子（order -8）中初始化，统一日志格式和输出目标
 2. 环境自适应：SIT 环境输出 Debug 级别并附带调用方信息，生产环境输出 Info 级别
 3. 文件轮转：基于 lumberjack，按大小滚动、超期清理、旧文件 gzip 压缩
 4. TraceId 传递：通过 context 携带 TraceId，Ctx(ctx) 取出的日志自动附带 traceId 字段
 5. 输出扩展：可选 Loki 输出（loki.go）和错误日志计数钩子（error.go）

输出策略:
  - sit/dev 环境：同时输出到 stdout（控制台）和文件（./logs/application.log）
  - prd 环境：仅输出到文件，避免 stdout 被容器采集后与文件日志重复
*/
package logger

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

/*
init 注册全局 Logger 初始化钩子（order -8，晚于 env -10 / static -9）。

配置说明：
  - 时间格式精确到毫秒，便于排查时序问题
  - Caller 保留文件路径最后 3 级 + 行号（见 splitFilePath）
  - prd 环境：Info 级别，仅写文件；其他环境：Debug 级别，写 stdout + 文件
  - 固定字段：ip（本机 IP，用于多实例日志区分）
  - logger.loki.enable 开启时追加 Loki 输出（见 loki.go）
  - 错误日志计数钩子始终挂载（见 error.go），Error/Fatal/Panic 级别日志会计入 Prometheus Counter

注意：env.LocalIP、static 配置由各自更小的 order（-10/-9）保证先就绪。
*/
func init() {
	start.AddInit(func() {
		zerolog.TimeFieldFormat = "2006-01-02 15:04:05.000"
		zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
			return splitFilePath(file) + ":" + strconv.Itoa(line)
		}

		var out io.Writer
		rotateLogger := newRotateLogger()
		quit.AddFinalShutdownHook(func() {
			rotateLogger.Close()
		})
		switch env.Env {
		case env.PrdEnv:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			out = rotateLogger
		case env.DebugEnv:
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
			out = io.MultiWriter(os.Stdout, rotateLogger)
		default:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			out = io.MultiWriter(os.Stdout, rotateLogger)
		}

		enableLoki := static.GetBool("logger.loki.enable")
		if enableLoki {
			out = newLokiWriter(out)
		}

		logger := zerolog.New(out).With().Str("ip", env.LocalIP).Timestamp()

		if env.IsSitEnv() || env.IsDebugEnv() {
			logger = logger.Caller()
		}

		Logger = logger.Logger().Hook(newErrorHook())
	}, -8)
}

/*
Logger 全局日志实例，供整个应用使用。

直接使用:

	logger.Logger.Info().Msg("server started")
	logger.Logger.Error().Err(err).Msg("failed to connect db")

请求链路内建议使用 Ctx(ctx)，自动附带 traceId 字段。
*/
var Logger zerolog.Logger

/*
Ctx 返回带 traceId 字段的子 Logger，traceId 取自 context（见 GetTraceId）。

	func Handle(ctx context.Context) {
	    logger.Ctx(ctx).Info().Msg("request received")             // traceId=abc123
	    logger.Ctx(ctx).Error().Err(err).Msg("processing failed")  // traceId=abc123
	}

每次调用都会新建 Logger，高频调用建议在函数开头获取一次后复用。
context 中无 TraceId 时 traceId 字段为 "-"。
*/
func Ctx(ctx context.Context) *zerolog.Logger {
	c := Logger.With().Ctx(ctx).Str("traceId", GetTraceId(ctx))
	l := c.Logger()
	return &l
}

/*
newRotateLogger 创建日志文件切割器（lumberjack）。

轮转参数：
  - 单文件超过 100MB 后切换新文件
  - 保留最近 10 个备份文件
  - 保留 20 天内的文件，超期删除
  - 旧文件 gzip 压缩（.gz 后缀）

lumberjack.Logger 并发安全，可直接作为 zerolog 的输出目标。
*/
func newRotateLogger() *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   "./logs/application.log",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     20,
		Compress:   true,
	}
}

/*
splitFilePath 裁剪文件路径，保留最后 3 级，避免 monorepo 下 zerolog 默认输出的超长绝对路径。

	/Users/lizexin/GoProjects/dsp/services/admin/internal/user/service.go
	→ services/admin/internal/user/service.go

	short.go → short.go  （不足 3 级时原样返回）
*/
func splitFilePath(path string) string {
	split := strings.Split(path, string(filepath.Separator))
	if len(split) < 3 {
		return path
	}
	i := len(split)
	return filepath.Join(split[i-3], split[i-2], split[i-1])
}
