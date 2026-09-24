package logger

import (
	"fmt"

	"github.com/VictoriaMetrics/metrics"
	"github.com/rs/zerolog"
)

/*
本文件实现 zerolog.Hook：错误日志计数打点。

设计要点：
  - Error/Fatal/Panic 级别各累加一个 Prometheus Counter，
    指标名 log_error_total，标签 level 区分级别
  - 计数由 VictoriaMetrics metrics 包维护（内部并发安全），
    随 promhelper 的 push 任务统一推送到远端，无需额外的告警聚合逻辑
  - Fatal/Panic 日志意味着进程即将退出，Run 内计数后同步触发一次指标冲刷
    （AddFatalFlush 注册的回调），避免退出前的计数丢失
*/

/*
errorHook 错误日志计数钩子，实现 zerolog.Hook 接口。

无状态结构体：计数直接写入 metrics 全局注册表，钩子本身不保存字段。
*/
type errorHook struct{}

/*
newErrorHook 创建错误日志计数钩子。
*/
func newErrorHook() zerolog.Hook {
	return new(errorHook)
}

/*
fatalFlush Fatal/Panic 日志的即时冲刷回调，由 AddFatalFlush 注册。

此类日志意味着进程即将退出（Fatal → os.Exit(1)，Panic → panic 展开），
仅靠周期推送必然丢失退出前的计数，故在 Hook 内同步冲刷一次。
未注册时为 nil（push 模式未启用），Run 中跳过。
*/
var fatalFlush []func()

/*
AddFatalFlush 注册 Fatal/Panic 日志的即时冲刷回调。

由 promhelper.EnablePushTask 在启动阶段注册（logger 不能反向 import promhelper，
故采用注册点解耦），运行期不应变更。

回调在日志调用方 goroutine 内同步执行，进程即将退出，短暂阻塞可接受，
但实现方需自带超时保护（如 promhelper 中的 5 秒 ctx 超时）。
*/
func AddFatalFlush(fn func()) {
	fatalFlush = append(fatalFlush, fn)
}

/*
Run 实现 zerolog.Hook 接口，每条日志输出时被 zerolog 回调。

Error/Fatal/Panic 级别各对应一个独立 Counter（level 标签区分），其他级别忽略。

Fatal/Panic 级别计数后同步执行 fatalFlush 冲刷（若已注册）——zerolog 先执行
Hook 回调、后执行 os.Exit(1)/panic（见 zerolog Event.msg），此时冲刷仍来得及。

注意：不可简化为 level >= zerolog.ErrorLevel —— NoLevel(6)/DisabledLevel(7)
数值上大于 PanicLevel(5)，会被误计入。
*/
func (h *errorHook) Run(_ *zerolog.Event, level zerolog.Level, _ string) {
	if level != zerolog.ErrorLevel && level != zerolog.FatalLevel && level != zerolog.PanicLevel {
		return
	}
	metrics.GetOrCreateCounter(
		fmt.Sprintf("log_error_total{level=%q}", level.String()),
	).Inc()
	if (level == zerolog.FatalLevel || level == zerolog.PanicLevel) && len(fatalFlush) > 0 {
		for _, fn := range fatalFlush {
			fn()
		}
	}
}
