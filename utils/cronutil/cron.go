// Package cronutil 封装 robfig/cron/v3，提供定时任务注册与 cron 表达式解析，
// 并预置常用 cron 表达式与时间格式常量。
// 注意：表达式为 5 段格式（分 时 日 月 周），不含秒。
package cronutil

import (
	"time"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"

	"github.com/robfig/cron/v3"
)

// 预置的常用 cron 表达式（5 段格式：分 时 日 月 周）与日期时间格式常量。
// 其中 "?" 与 "*" 等价（本库解析器将 ? 视为 *），便于照搬 Quartz 表达式。
const (
	EveryStartOfDay = "0 0 * * ?"

	EveryHourAtMinute1 = "1 * * * *"
	EveryHourAtMinute2 = "2 * * * *"
	EveryHourAtMinute5 = "5 * * * *"

	EveryDayAt1am = "0 1 * * *"
	EveryDayAt2am = "0 2 * * *"
	EveryDayAt5am = "0 5 * * *"

	EveryMinute   = "* * * * *"
	Every5Minutes = "*/5 * * * *"

	EveryMondayAt3am   = "0 3 * * 1"
	EveryMonth1stAt3am = "0 3 1 * ?"

	YearFormat   = "2006"
	MonthFormat  = "200601"
	DateFormat   = "20060102"
	HourFormat   = "2006010215"
	MinuteFormat = "200601021504"
)

var (
	parser cron.Parser
	manage *cron.Cron
)

func init() {
	start.AddInit(func() {
		parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		manage = cron.New(
			cron.WithParser(parser),
			cron.WithLogger(cron.DiscardLogger),
			cron.WithLocation(time.Local),
		)
		manage.Start()
		quit.AddApplicationShutdownHook(func() {
			ctx := manage.Stop()
			<-ctx.Done()
		})
	}, -7)
}

// Parse 解析 5 段 cron 表达式，返回调度计划（Schedule）；表达式非法时返回 error。
func Parse(expr string) (cron.Schedule, error) {
	return parser.Parse(expr)
}

// AddFunc 注册定时任务：按 spec 表达式周期执行 fn。
// 注意：表达式非法时直接 Fatal 退出进程（而非返回错误），调用前应先用 Parse 校验。
func AddFunc(spec string, fn func()) {
	_, err := manage.AddFunc(spec, fn)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("")
	}
}

// AddLoop 启动一个常驻 goroutine，每间隔 duration 执行一次 fn。
// 注意：先执行 fn 再等待，间隔从 fn 返回后开始计算（非固定周期调度）；
// 收到退出信号后（quit.Stopping）在当前等待结束即退出，不再开启下一轮。
func AddLoop(duration time.Duration, fn func()) {
	go func() {
		for {
			fn()
			select {
			case <-quit.Stopping():
				return
			case <-time.After(duration):
			}
		}
	}()
}
