package xormlog

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/property/loader"
	"time"
	xormlog "xorm.io/xorm/log"
)

var (
	showSql          bool
	XormReportLogger xormlog.ContextLogger
	slowSqlDuration  time.Duration
)

func init() {
	showSql = property.GetBool("xorm.ShowSql")
	XormReportLogger = &xLogger{
		DiscardLogger: xormlog.DiscardLogger{},
	}
	duration := property.GetInt64("xorm.slowSqlDuration")
	if duration > 0 {
		slowSqlDuration = time.Duration(duration) * time.Millisecond
	}
	loader.OnKeyChange("xorm.ShowSql", func() {
		showSql = property.GetBool("xorm.ShowSql")
	})
}

// xLogger 实现xorm sql的prometheus上报
type xLogger struct {
	xormlog.DiscardLogger
}

func (x *xLogger) IsShowSQL() bool {
	return true
}

func (x *xLogger) BeforeSQL(xormlog.LogContext) {}

func (x *xLogger) AfterSQL(ctx xormlog.LogContext) {
	if showSql {
		logger.Logger.WithContext(ctx.Ctx).Infof("[SQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
	if slowSqlDuration > 0 && ctx.ExecuteTime > slowSqlDuration {
		logger.Logger.WithContext(ctx.Ctx).Errorf("[SlowSQL] %s %v - %v", ctx.SQL, ctx.Args, ctx.ExecuteTime)
	}
}
