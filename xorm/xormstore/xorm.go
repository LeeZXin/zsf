package xormstore

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/xorm/xormutil"
	_ "github.com/go-sql-driver/mysql"
	"time"
	"xorm.io/xorm"
)

var (
	engine *xormutil.Engine
)

func init() {
	var err error
	engine, err = xormutil.NewEngine(xormutil.Config{
		DataSourceName:  static.GetString("xorm.dataSourceName"),
		MaxIdleConns:    static.GetInt("xorm.maxIdleConns"),
		ConnMaxLifetime: static.GetInt("xorm.connMaxLifetime"),
		MaxOpenConns:    static.GetInt("xorm.maxOpenConns"),
		ShowSql:         static.GetBool("xorm.showSql"),
		SlowSqlDuration: time.Duration(static.GetInt("xorm.slowSqlDuration")) * time.Millisecond,
	})
	if err != nil {
		logger.Logger.Fatalf("mysqlstore.xorm init failed: %v", err)
	}
}

func TxContext(pctx context.Context) (context.Context, xormutil.Committer, error) {
	return engine.TxContext(pctx)
}

func WithTx(ctx context.Context, fn func(context.Context) error) error {
	return engine.WithTx(ctx, fn)
}

func Context(ctx context.Context) (context.Context, xormutil.Closer) {
	return engine.Context(ctx)
}

func GetXormSession(ctx context.Context) *xorm.Session {
	return engine.GetXormSession(ctx)
}

func NewXormSession(ctx context.Context) *xorm.Session {
	return engine.NewXormSession(ctx)
}

func GetEngine() *xorm.Engine {
	return engine.GetEngine()
}
