package xormutil

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/xorm/xormlog"
	_ "github.com/go-sql-driver/mysql"
	"strings"
	"time"
	"xorm.io/xorm"
)

var (
	enginedContextKey = &contextKey{"x"}
)

type contextKey struct {
	name string
}

type xormContext struct {
	context.Context
	session *xorm.Session
}

func (ctx *xormContext) Value(key any) any {
	if key == enginedContextKey {
		return ctx
	}
	return ctx.Context.Value(key)
}

type Committer interface {
	Commit() error
	Rollback() error
	Closer
}

type Closer interface {
	Close() error
}

type xormCommitter struct {
	session *xorm.Session
}

func (c *xormCommitter) Commit() error {
	return c.session.Commit()
}

func (c *xormCommitter) Rollback() error {
	return c.session.Rollback()
}

func (c *xormCommitter) Close() error {
	return c.session.Close()
}

type xormCloser struct {
	session *xorm.Session
}

func (c *xormCloser) Close() error {
	return c.session.Close()
}

type discardCloser struct {
}

func (c *discardCloser) Close() error {
	return nil
}

type Config struct {
	DataSourceName  string `json:"dataSourceName"`
	MaxIdleConns    int    `json:"maxIdleConns"`
	ConnMaxLifetime int    `json:"connMaxLifetime"`
	MaxOpenConns    int    `json:"maxOpenConns"`
	ShowSql         bool   `json:"showSql"`
	SlowSqlDuration time.Duration
}

type Engine struct {
	engine *xorm.Engine
}

func NewEngine(config Config) (*Engine, error) {
	engine, err := xorm.NewEngine("mysql", config.DataSourceName)
	if err != nil {
		return nil, err
	}
	maxIdleConns := config.MaxIdleConns
	if maxIdleConns > 0 {
		engine.SetMaxIdleConns(maxIdleConns)
	}
	connMaxLifetime := config.ConnMaxLifetime
	if connMaxLifetime > 0 {
		engine.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
	}
	maxOpenConns := config.MaxOpenConns
	if maxOpenConns > 0 {
		engine.SetMaxOpenConns(maxOpenConns)
	}
	engine.SetLogger(xormlog.NewXLogger(config.ShowSql, config.SlowSqlDuration))
	quit.AddShutdownHook(func() {
		_ = engine.Close()
	})
	return &Engine{
		engine: engine,
	}, nil
}

func (*Engine) newContext(ctx context.Context, session *xorm.Session) *xormContext {
	return &xormContext{
		Context: ctx,
		session: session,
	}
}

func (e *Engine) TxContext(ctx context.Context) (context.Context, Committer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session := e.getTxXormSession(ctx)
	if session.IsInTx() {
		return ctx, &xormCommitter{session: session}, nil
	}
	if err := session.Begin(); err != nil {
		return ctx, nil, err
	}
	return e.newContext(ctx, session), &xormCommitter{session: session}, nil
}

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
		return err
	}
	return committer.Commit()
}

func (e *Engine) Context(ctx context.Context) (context.Context, Closer) {
	if ctx == nil {
		ctx = context.Background()
	}
	if xctx, ok := ctx.(*xormContext); ok {
		if !xctx.session.IsClosed() {
			return xctx, &discardCloser{}
		}
	}
	session := e.NewXormSession(ctx)
	return e.newContext(ctx, session), &xormCloser{session: session}
}

func (e *Engine) GetXormSession(ctx context.Context) *xorm.Session {
	if ctx != nil {
		if xctx, ok := ctx.(*xormContext); ok {
			if !xctx.session.IsClosed() {
				return xctx.session
			}
		}
	}
	return e.newAutoCloseXormSession(ctx)
}

func (e *Engine) getTxXormSession(ctx context.Context) *xorm.Session {
	if ctx != nil {
		if xctx, ok := ctx.(*xormContext); ok {
			if !xctx.session.IsClosed() {
				return xctx.session
			}
		}
	}
	return e.NewXormSession(ctx)
}

func (e *Engine) NewXormSession(ctx context.Context) *xorm.Session {
	session := e.engine.NewSession()
	session.Context(ctx)
	return session
}

func (e *Engine) newAutoCloseXormSession(ctx context.Context) *xorm.Session {
	return e.engine.Context(ctx)
}

func (e *Engine) GetEngine() *xorm.Engine {
	return e.engine
}

func MustGetXormSession(ctx context.Context) *xorm.Session {
	if ctx != nil {
		if xctx, ok := ctx.(*xormContext); ok {
			if !xctx.session.IsClosed() {
				return xctx.session
			}
		}
	}
	logger.Logger.WithContext(ctx).Panic("failed to get xorm.Session")
	return nil
}

func IsDuplicatedEntryError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Error 1062")
}
