// Package xormdb 封装 xorm 引擎与会话管理：数据库会话（*xorm.Session）经
// context 传递，业务代码无需关心 session 的创建/关闭细节。
//
// 用法约定：
//   - 查询/写操作统一通过 Engine.Context / GetSession 获取会话；Context 返回
//     的 Closer 用后必关（事务场景返回空操作 Closer，见 Context 注释）
//   - 事务必须用 TxContext / WithTx 开启；事务内通过 Context/GetSession 拿到的
//     是同一会话，嵌套 WithTx 自动并入外层事务（不重复开启、不提前提交）
//   - 同一 session 不可多 goroutine 并发使用（xorm 限制，且框架不提供
//     session 级并发保护）
//   - 会话经 context 传递：Context/TxContext 把 session 写入 context.Value，
//     后续 WithValue/WithTimeout/WithCancel 包装后 GetSession 仍能取到同一会话
//
// 支持 mysql 与 sqlite3 两种驱动；sqlite3 驱动下会自动创建数据文件所在目录。
// 包级快捷入口见 xormdb/database。
package xormdb

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/LeeZXin/zsf/logger"

	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL 驱动
	_ "github.com/mattn/go-sqlite3"    // SQLite3 驱动
	"xorm.io/xorm"
)

// 支持的数据库驱动名：Mysql 与 Sqlite3；Config.Driver 为空时默认使用 Mysql。
const (
	Mysql   = "mysql"
	Sqlite3 = "sqlite3"
)

// ContextKey context中key的结构体定义
type ContextKey struct {
	name string
}

// sessionContext xorm上下文封装，包含原始context和xorm session
// 用于在context中传递xorm session以便在事务中使用
type sessionContext struct {
	context.Context
	contextKey *ContextKey
	session    *xorm.Session
}

// sessionCtxLookupKey 让包装后的 context（WithValue/WithTimeout）仍能取到会话。
// 与 per-engine contextKey 并存：GetSession 用引擎自己的 key，MustGetSession 用本 key。
var sessionCtxLookupKey = &ContextKey{name: "xormdb.session"}

// Value 实现context.Context接口，从context中获取xorm session
func (ctx *sessionContext) Value(key any) any {
	if key == ctx.contextKey || key == sessionCtxLookupKey {
		return ctx
	}
	return ctx.Context.Value(key)
}

// Committer 事务提交器接口，包含提交、回滚和关闭操作
type Committer interface {
	Commit() error
	Rollback() error
	Closer
}

// Closer 关闭接口
type Closer interface {
	Close() error
}

// sessionCommitter xorm事务提交器实现
type sessionCommitter struct {
	session *xorm.Session
}

// Commit 提交事务
func (c *sessionCommitter) Commit() error {
	return c.session.Commit()
}

// Rollback 回滚事务
func (c *sessionCommitter) Rollback() error {
	return c.session.Rollback()
}

// Close 关闭session
func (c *sessionCommitter) Close() error {
	return c.session.Close()
}

// sessionCloser xorm session关闭器实现
type sessionCloser struct {
	session *xorm.Session
}

// Close 关闭xorm session
func (c *sessionCloser) Close() error {
	return c.session.Close()
}

// discardCloser 空关闭器，用于不需要关闭session的场景
type discardCloser struct {
}

func (c *discardCloser) Close() error {
	return nil
}

// nopCommitter 嵌套事务提交器：外层已持有事务时，内层 Commit/Rollback/Close 均为空操作，
// 避免内层 WithTx 提前提交或关闭外层会话。
type nopCommitter struct{}

func (c *nopCommitter) Commit() error   { return nil }
func (c *nopCommitter) Rollback() error { return nil }
func (c *nopCommitter) Close() error    { return nil }

// Config 数据库引擎配置结构
type Config struct {
	Driver          string        // 数据库驱动名称，默认为mysql
	Datasource      string        // 数据源名称（连接字符串）
	MaxIdleConns    int           // 最大空闲连接数
	ConnMaxLifetime int           // 连接最大生存时间（秒）
	MaxOpenConns    int           // 最大打开连接数
	ShowSql         bool          // 是否显示SQL语句
	SlowSqlDuration time.Duration // 慢查询阈值
	ContextKey      *ContextKey
}

// Engine 数据库引擎封装，提供事务和上下文管理
type Engine struct {
	engine     *xorm.Engine
	contextKey *ContextKey
}

// NewEngine 创建数据库引擎实例
// config: 数据库配置信息
// 返回配置好的Engine实例和错误信息
func NewEngine(config Config) (*Engine, error) {
	driver := config.Driver
	if driver == "" {
		driver = Mysql
	}
	datasource := config.Datasource
	if driver == Sqlite3 {
		if config.Datasource == "" {
			datasource = "./data/sqlite3.db"
		}
		err := os.MkdirAll(filepath.Dir(datasource), os.ModePerm)
		if err != nil {
			return nil, err
		}
	}
	db, err := xorm.NewEngine(driver, datasource)
	if err != nil {
		return nil, err
	}
	// 日志在前，ping会打印日志
	db.SetLogger(newXLogger(config.ShowSql, config.SlowSqlDuration))
	// ping一下 测试连通性
	err = db.Ping()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	db.DatabaseTZ = time.Local // 必须
	db.TZLocation = time.Local // 必须
	maxIdleConns := config.MaxIdleConns
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	connMaxLifetime := config.ConnMaxLifetime
	if connMaxLifetime > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
	}
	maxOpenConns := config.MaxOpenConns
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	contextKey := config.ContextKey
	if contextKey == nil {
		contextKey = &ContextKey{name: "xorm"}
	}
	return &Engine{
		engine:     db,
		contextKey: contextKey,
	}, nil
}

// newContext 创建xorm上下文
func (e *Engine) newContext(ctx context.Context, session *xorm.Session) *sessionContext {
	return &sessionContext{
		Context:    ctx,
		session:    session,
		contextKey: e.contextKey,
	}
}

// TxContext 获取事务上下文
// 如果context中已存在事务，则返回该事务的committer
// 否则创建新事务并返回
// 返回值：context上下文、事务提交器、错误信息
func (e *Engine) TxContext(ctx context.Context) (context.Context, Committer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session := e.getTxSession(ctx)
	if session.IsInTx() {
		return ctx, &nopCommitter{}, nil
	}
	if err := session.Begin(); err != nil {
		return ctx, nil, err
	}
	return e.newContext(ctx, session), &sessionCommitter{session: session}, nil
}

// WithTx 在事务中执行函数
// fn: 需要在事务中执行的函数，如果返回错误则自动回滚
// 自动管理事务的开启、提交和关闭
func (e *Engine) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("nil fn")
	}
	txContext, committer, err := e.TxContext(ctx)
	if err != nil {
		return err
	}
	defer committer.Close()
	err = fn(txContext)
	if err != nil {
		_ = committer.Rollback()
		return err
	}
	return committer.Commit()
}

// Context 获取数据库操作上下文
// 返回的context可用于执行数据库操作
// 返回的Closer使用完毕后需要调用Close
func (e *Engine) Context(ctx context.Context) (context.Context, Closer) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.sessionFromContext(ctx) != nil {
		return ctx, &discardCloser{}
	}
	session := e.NewSession(ctx)
	return e.newContext(ctx, session), &sessionCloser{session: session}
}

// GetSession 从context中获取xorm Session
// 如果context中存在有效session则返回，否则创建新的自动关闭session
func (e *Engine) GetSession(ctx context.Context) *xorm.Session {
	if s := e.sessionFromContext(ctx); s != nil {
		return s
	}
	return e.newAutoCloseSession(ctx)
}

// getTxSession 获取事务中的session
// 如果context中存在未关闭的session则返回，否则创建新session
func (e *Engine) getTxSession(ctx context.Context) *xorm.Session {
	if s := e.sessionFromContext(ctx); s != nil {
		return s
	}
	return e.NewSession(ctx)
}

func (e *Engine) sessionFromContext(ctx context.Context) *xorm.Session {
	if ctx == nil || e.contextKey == nil {
		return nil
	}
	v := ctx.Value(e.contextKey)
	if v == nil {
		return nil
	}
	xctx, ok := v.(*sessionContext)
	if !ok || xctx.session.IsClosed() {
		return nil
	}
	return xctx.session
}

// NewSession 创建新的xorm Session
// 需要手动管理session的生命周期
func (e *Engine) NewSession(ctx context.Context) *xorm.Session {
	session := e.engine.NewSession()
	session.Context(ctx)
	return session
}

// newAutoCloseSession 创建自动关闭的xorm Session
// session会在context结束时自动关闭
func (e *Engine) newAutoCloseSession(ctx context.Context) *xorm.Session {
	return e.engine.Context(ctx)
}

// GetEngine 返回原生 *xorm.Engine 实例，供需要原生能力的场景使用
// （如建表 Sync、全局配置）；绕过本包的会话管理时需自行负责
// session/连接的生命周期。
func (e *Engine) GetEngine() *xorm.Engine {
	return e.engine
}

// Close 关闭底层数据库连接池，释放所有空闲连接。
// xormdb/database 已注册退出时的关闭钩子，业务一般无需手动调用；
// 引擎关闭后所有会话操作都会失败。
func (e *Engine) Close() {
	e.engine.Close()
}

// MustGetSession 从 ctx 获取数据库会话；ctx 中不存在有效会话（既不在
// 事务中、也未通过 Context 获取过会话）时直接 Panic。
// 用于"必须在事务/会话内执行"的强约束场景：防止代码路径漏掉事务导致
// 在无会话状态下执行 SQL；调用前需确保 ctx 来自 WithTx / Context。
func MustGetSession(ctx context.Context) *xorm.Session {
	if ctx != nil {
		if v := ctx.Value(sessionCtxLookupKey); v != nil {
			if xctx, ok := v.(*sessionContext); ok && !xctx.session.IsClosed() {
				return xctx.session
			}
		}
	}
	logger.Logger.Panic().Msg("failed to get xorm.Session")
	return nil
}
