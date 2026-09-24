// Package database 提供 xormdb 的包级快捷入口：基于静态配置（xorm.driver、
// xorm.datasource、xorm.show-sql、xorm.slow-sql-duration）创建全局数据库引擎，
// 并导出事务/会话操作的薄封装函数。
//
// 引擎在 init 中创建，配置缺失或连接初始化失败直接 Fatal 退出进程；
// 退出时注册低优先级 shutdown hook 关闭连接池。业务代码（含 corelet 各
// repository）统一通过本包函数获取会话与事务，不要自行创建引擎。
package database

import (
	"context"
	"path/filepath"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/xormdb"

	"xorm.io/xorm"
)

var (
	// engine 全局数据库引擎实例
	engine *xormdb.Engine
)

// init 注册数据库引擎创建钩子（order -4，依赖 static -9/logger -8），
// 根据配置文件创建引擎并注册关闭钩子。
func init() {
	start.AddInit(func() {
		if err := Init(); err != nil {
			logger.Logger.Fatal().Msgf("xorm init failed: %v", err)
		}
	}, -4)
}

// Init 按静态配置创建全局引擎。已创建则直接返回。
// sqlite 相对路径相对 static.Dir()，避免 git-hook 的 cwd 落在裸仓里连错库。
func Init() error {
	if engine != nil {
		return nil
	}
	driver := static.GetString("xorm.driver")
	ds := static.GetString("xorm.datasource")
	if driver == xormdb.Sqlite3 {
		if ds == "" {
			ds = filepath.Join("data", "sqlite3.db")
		}
		if !filepath.IsAbs(ds) {
			if base := static.Dir(); base != "" {
				ds = filepath.Join(base, ds)
			}
		}
	}
	var err error
	engine, err = xormdb.NewEngine(xormdb.Config{
		Driver:          driver,
		Datasource:      ds,
		ShowSql:         static.GetBool("xorm.show-sql"),
		SlowSqlDuration: static.GetDuration("xorm.slow-sql-duration"),
	})
	if err != nil {
		return err
	}
	quit.AddLowPriorityShutdownHook(engine.Close)
	return nil
}

// TxContext 获取带事务的 context。
// 如果 context 中已存在事务则复用，否则创建新事务。
// 返回: 包含事务的 context、事务提交器和错误信息
func TxContext(ctx context.Context) (context.Context, xormdb.Committer, error) {
	return engine.TxContext(ctx)
}

// WithTx 在事务中执行指定函数。
// 函数执行失败时自动回滚事务，成功时自动提交事务。
// fn: 需要在事务中执行的函数
func WithTx(ctx context.Context, fn func(context.Context) error) error {
	return engine.WithTx(ctx, fn)
}

// Context 获取数据库操作上下文。
// 返回的 context 可用于执行数据库操作，使用完毕后需要调用返回的 Closer 的 Close 方法。
func Context(ctx context.Context) (context.Context, xormdb.Closer) {
	return engine.Context(ctx)
}

// GetSession 从 context 中获取 xorm Session。
// 如果 context 中存在有效 session 则复用，否则创建新的 session。
func GetSession(ctx context.Context) *xorm.Session {
	return engine.GetSession(ctx)
}

// NewSession 创建新的 xorm Session。
// 需要手动管理 session 的生命周期。
func NewSession(ctx context.Context) *xorm.Session {
	return engine.NewSession(ctx)
}

// GetEngine 获取原生 xorm.Engine 实例。
func GetEngine() *xorm.Engine {
	return engine.GetEngine()
}
