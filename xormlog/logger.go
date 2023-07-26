package xormlog

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/propertywatcher"
	"time"
	log "xorm.io/xorm/log"
)

var (
	showSql          bool
	XormReportLogger log.ContextLogger
	slowSqlDuration  time.Duration
)

func init() {
	showSqlKey := "xorm.showSql"
	slowSqlDurationKey := "xorm.slowSqlDuration"
	showSql = property.GetBool(showSqlKey)
	XormReportLogger = &xLogger{
		DiscardLogger: log.DiscardLogger{},
	}
	duration := property.GetInt64(slowSqlDurationKey)
	if duration > 0 {
		slowSqlDuration = time.Duration(duration) * time.Millisecond
	}
	propertywatcher.OnKeyChange(showSqlKey, func() {
		showSql = property.GetBool(showSqlKey)
	})
}

// xLogger 实现xorm sql的日志告警
type xLogger struct {
	log.DiscardLogger
}

func (x *xLogger) IsShowSQL() bool {
	return true
}

func (x *xLogger) BeforeSQL(log.LogContext) {}

func (x *xLogger) AfterSQL(ctx log.LogContext) {
	if showSql {
		logger.Logger.WithContext(ctx.Ctx).Infof("[SQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
	if slowSqlDuration > 0 && ctx.ExecuteTime > slowSqlDuration {
		logger.Logger.WithContext(ctx.Ctx).Errorf("[SlowSQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
}
