/*
Package promhelper 提供基于 VictoriaMetrics 的 Prometheus 指标埋点。

与 prometheus/client_golang 相比，VictoriaMetrics 的优势：
  - 更低的内存占用和更快的序列化
  - 内置 push 模式支持（定期将指标推送到 VictoriaMetrics / Prometheus Pushgateway）
  - Summary 类型自动计算分位数，无需预定义 bucket

指标类型：
  - http_server_request_total:  HTTP 请求监控（标签: request, code）
  - grpc_server_request_total: gRPC 服务端请求监控（标签: request）
  - grpc_client_request_total: gRPC 客户端请求监控（标签: target, method）

所有指标使用 Summary 类型，记录请求耗时分布（UpdateDuration）。

Push 模式：

	通过 EnablePushTask() 启用，定期将本地指标推送到 prom.push.url 指定的服务。
	推送标签 service_name（应用名）和 ip（本机 IP）用于区分不同服务和实例。

注意：
  - /metrics 路径的请求被 HttpServerRequestTotal 自动跳过，不统计自身
  - push URL 为空时 EnablePushTask() 会 Fatal 退出（push 模式是必需的）
*/
package promhelper

import (
	"context"
	"fmt"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
	"github.com/VictoriaMetrics/metrics"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

/*
HttpServerRequestTotal 记录 HTTP 服务端请求指标。

参数：
  - request:   请求的 URL 路径
  - code:      HTTP 响应状态码
  - startTime: 请求开始时间（time.Now()），函数内部计算耗时

指标名格式: http_server_request_total{request="/api/user/list", code="200"}

metrics 路径的请求会被跳过（if request == "/metrics"），避免自引用。
*/
func HttpServerRequestTotal(request string, code int, startTime time.Time) {
	if request == "/metrics" {
		return
	}
	metrics.GetOrCreateSummary(
		fmt.Sprintf(`http_server_request_total{request=%q,code="%d"}`, request, code),
	).UpdateDuration(startTime)
}

/*
GrpcClientRequestTotal 记录 gRPC 客户端请求指标。

参数：
  - target:    目标服务名（如 "dsp-admin"）
  - method:    RPC 方法全路径（如 "/dsp.admin.UserService/Login"）
  - startTime: 调用开始时间

指标名格式: grpc_client_request_total{target="dsp-admin", method="/dsp.admin.UserService/Login"}
*/
func GrpcClientRequestTotal(target, method string, startTime time.Time) {
	metrics.GetOrCreateSummary(
		fmt.Sprintf(`grpc_client_request_total{target=%q,method=%q}`, target, method),
	).UpdateDuration(startTime)
}

/*
GrpcServerRequestTotal 记录 gRPC 服务端请求指标。

参数：
  - request:   RPC 方法全路径
  - startTime: 请求开始时间

指标名格式: grpc_server_request_total{request="/dsp.admin.UserService/Login"}
*/
func GrpcServerRequestTotal(request string, startTime time.Time) {
	if request == healthpb.Health_Check_FullMethodName {
		return
	}
	metrics.GetOrCreateSummary(
		fmt.Sprintf(`grpc_server_request_total{request=%q}`, request),
	).UpdateDuration(startTime)
}

/*
ExtraLabels 所有指标共用的推送标签，用于区分不同服务和实例：
service_name 取自 application.name，ip 取自 env.LocalIP。
由 start 钩子（order -6，晚于 instance/env）在启动时填充。
*/
var (
	ExtraLabels string
)

func init() {
	start.AddInit(func() {
		ExtraLabels = fmt.Sprintf(`service_name=%q,ip=%q`, instance.ApplicationName, env.LocalIP)
	})
}

/*
EnablePushTask 启动指标推送任务。

定期将本地积攒的指标数据 Push 到 prom.push.url 配置的远端服务。

配置项（在 application-{env}.yaml 中):

	prom.push.url:      推送目标地址（必需，为空则 Fatal）。
	                    **必须写到导入端点**，库不会自动补路径——VictoriaMetrics 单机版要写成
	                    http://127.0.0.1:8428/api/v1/import/prometheus；只写到主机端口会被它
	                    以 `unsupported path requested: "/"` 400 掉。指向 Pushgateway 时同理，
	                    写它的完整接收路径。

标签说明:

	service_name:    application.name（如 "dsp-admin"），区分不同服务
	ip:  env.LocalIP（如 "10.0.0.1"），区分同一服务的不同实例

退出冲刷:

	两级冲刷机制，避免进程退出前积攒的指标（如 log_error_total 计数）丢失：

	1. Fatal/Panic 即时冲刷：通过 logger.AddFatalFlush 注册，
	   在 zerolog Hook 内同步 Push（早于 os.Exit(1)/panic），
	   覆盖 Fatal/Panic 退出路径（见 logger/error.go）
	2. 优雅退出冲刷：注册 quit Final 关闭钩子，收到 SIGINT/SIGTERM 后
	   同步 Push 一次，覆盖常规退出路径

	两者均全量推送（绝对值），与周期任务并发或重复执行无害。
	单次 5 秒超时，推送失败仅记日志不阻塞退出。

Push 模式 vs Pull 模式:
  - Pull 模式要求 Prometheus 能访问每个实例的 /metrics 端点
  - Push 模式适合短生命周期任务、K8s 环境（实例 IP 动态变化）
  - 本框架选择 Push 模式，通过 VictoriaMetrics 的 InitPush 实现
*/
func EnablePushTask() {
	pushUrl := static.GetString("prom.push.url")
	if pushUrl == "" {
		logger.Logger.Fatal().Msg("empty prom.push.url")
	}
	logger.Logger.Info().Msgf("enable victoria metrics push task with url: %s", pushUrl)
	err := metrics.InitPush(pushUrl, 5*time.Second, ExtraLabels, true)
	if err != nil {
		logger.Logger.Fatal().Msgf("init metrics.pushTask failed with err: %v", err)
	}
	flush := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metrics.PushMetrics(ctx, pushUrl, true, &metrics.PushOptions{ExtraLabels: ExtraLabels}); err != nil {
			// InitPush 的后台任务每 5 秒失败一次，但那个失败只走标准库 log（stderr），
			// 不进 zsf 日志，配错 URL 会一直静默。这里至少让每次主动推送都留下痕迹。
			logger.Logger.Err(err).Msgf("push metrics to %s failed", pushUrl)
		}
	}
	// 先推一次：URL 写错（如漏了 /api/v1/import/prometheus）或对端不通时当场暴露，
	// 不用等后台任务慢慢失败
	flush()
	logger.AddFatalFlush(flush)
	quit.AddFinalShutdownHook(flush)
}
