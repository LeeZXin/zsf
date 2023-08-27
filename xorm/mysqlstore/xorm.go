package mysqlstore

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/xorm/mysqlutil"
	_ "github.com/go-sql-driver/mysql"
	"time"
	"xorm.io/xorm"
)

var (
	engine *mysqlutil.Engine
)

func init() {
	var err error
	engine, err = mysqlutil.NewEngine(mysqlutil.Config{
		DataSourceName:  property.GetString("xorm.dataSourceName"),
		MaxIdleConns:    property.GetInt("xorm.maxIdleConns"),
		ConnMaxLifetime: property.GetInt("xorm.connMaxLifetime"),
		MaxOpenConns:    property.GetInt("xorm.maxOpenConns"),
		ShowSql:         property.GetBool("xorm.showSql"),
		SlowSqlDuration: time.Duration(property.GetInt("xorm.slowSqlDuration")) * time.Millisecond,
	})
	if err != nil {
		logger.Logger.Panic(err)
	}
}

func TxContext(pctx context.Context) (context.Context, mysqlutil.Committer, error) {
	return engine.TxContext(pctx)
}

func WithTx(ctx context.Context, fn func(context.Context) error) error {
	return engine.WithTx(ctx, fn)
}

func Context(pctx context.Context) (context.Context, mysqlutil.Closer) {
	return engine.Context(pctx)
}

func GetXormSession(ctx context.Context) *xorm.Session {
	return engine.GetXormSession(ctx)
}

func NewXormSession(ctx context.Context) *xorm.Session {
	return engine.NewXormSession(ctx)
}
