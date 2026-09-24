package xormdb

import (
	"time"

	"github.com/LeeZXin/zsf/logger"

	"xorm.io/xorm/log"
)

// xLogger 实现xorm sql的日志告警
// xLogger wraps xorm's log.Logger to provide custom SQL logging and slow query alerting
type xLogger struct {
	log.DiscardLogger
	showSql         bool          // 是否显示SQL语句
	slowSqlDuration time.Duration // 慢查询阈值，超过该时间则记录为慢查询
}

// newXLogger 创建XLogger实例
// showSql: 是否打印SQL语句
// slowSqlDuration: 慢查询时间阈值，0表示不记录慢查询
func newXLogger(showSql bool, slowSqlDuration time.Duration) log.ContextLogger {
	return &xLogger{
		DiscardLogger:   log.DiscardLogger{},
		showSql:         showSql,
		slowSqlDuration: slowSqlDuration,
	}
}

// IsShowSQL 是否显示SQL
func (x *xLogger) IsShowSQL() bool {
	return true
}

// BeforeSQL 在SQL执行前调用（此处为空实现）
func (x *xLogger) BeforeSQL(log.LogContext) {}

// AfterSQL 在SQL执行后调用，用于记录SQL日志和慢查询告警
// ctx: 包含SQL语句、执行参数和执行时间等上下文信息
func (x *xLogger) AfterSQL(ctx log.LogContext) {
	// 如果开启SQL显示，则打印SQL语句
	if x.showSql {
		logger.Ctx(ctx.Ctx).Info().Msgf("[SQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
	// 如果设置了慢查询阈值且执行时间超过阈值，则记录慢查询错误
	if x.slowSqlDuration > 0 && ctx.ExecuteTime >= x.slowSqlDuration {
		logger.Ctx(ctx.Ctx).Error().Msgf("[SlowSQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
}
