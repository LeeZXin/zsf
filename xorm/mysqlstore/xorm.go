package mysqlstore

import (
	"context"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/static"
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
		DataSourceName:  static.GetString("xorm.dataSourceName"),
		MaxIdleConns:    static.GetInt("xorm.maxIdleConns"),
		ConnMaxLifetime: static.GetInt("xorm.connMaxLifetime"),
		MaxOpenConns:    static.GetInt("xorm.maxOpenConns"),
		ShowSql:         static.GetBool("xorm.showSql"),
		SlowSqlDuration: time.Duration(static.GetInt("xorm.slowSqlDuration")) * time.Millisecond,
	})
	if err != nil {
		logger.Logger.Panic(err)
	}
	logger.Logger.Infof("init mysqlstore: %s", static.GetString("xorm.dataSourceName"))
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

func GetEngine() *xorm.Engine {
	return engine.GetEngine()
}
