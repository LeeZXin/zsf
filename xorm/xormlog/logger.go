package xormlog

import (
	"github.com/LeeZXin/zsf/logger"
	"time"
	"xorm.io/xorm/log"
)

// XLogger 实现xorm sql的日志告警
type XLogger struct {
	log.DiscardLogger
	showSql         bool
	slowSqlDuration time.Duration
}

func NewXLogger(showSql bool, slowSqlDuration time.Duration) log.ContextLogger {
	return &XLogger{
		DiscardLogger:   log.DiscardLogger{},
		showSql:         showSql,
		slowSqlDuration: slowSqlDuration,
	}
}

func (x *XLogger) IsShowSQL() bool {
	return true
}

func (x *XLogger) BeforeSQL(log.LogContext) {}

func (x *XLogger) AfterSQL(ctx log.LogContext) {
	if x.showSql {
		logger.Logger.WithContext(ctx.Ctx).Infof("[SQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
	if x.slowSqlDuration > 0 && ctx.ExecuteTime >= x.slowSqlDuration {
		logger.Logger.WithContext(ctx.Ctx).Errorf("[SlowSQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
}
