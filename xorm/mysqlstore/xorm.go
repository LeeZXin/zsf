package mysqlstore

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/xorm/xormlog"
	_ "github.com/go-sql-driver/mysql"
	"time"
	"xorm.io/xorm"
)

var (
	engine            *xorm.Engine
	enginedContextKey = &contextKey{"x"}
)

type contextKey struct {
	name string
}

func init() {
	var err error
	engine, err = xorm.NewEngine("mysql", property.GetString("xorm.dataSourceName"))
	if err != nil {
		logger.Logger.Panic(err)
	}
	maxIdleConns := property.GetInt("xorm.maxIdleConns")
	if maxIdleConns > 0 {
		engine.SetMaxIdleConns(maxIdleConns)
	}
	connMaxLifetime := property.GetInt("xorm.connMaxLifetime")
	if connMaxLifetime > 0 {
		engine.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
	}
	maxOpenConns := property.GetInt("xorm.maxOpenConns")
	if maxOpenConns > 0 {
		engine.SetMaxOpenConns(maxOpenConns)
	}
	engine.SetLogger(xormlog.XormReportLogger)
	quit.AddShutdownHook(func() {
		_ = engine.Close()
	})
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

func (c *xormCommitter) Close() error {
	return c.session.Close()
}

type xormCloser struct {
	session *xorm.Session
}

func (c *xormCloser) Close() error {
	return c.session.Close()
}

func newContext(ctx context.Context, session *xorm.Session) *xormContext {
	return &xormContext{
		Context: ctx,
		session: session,
	}
}

func TxContext(pctx context.Context) (context.Context, Committer, error) {
	if pctx == nil {
		pctx = context.Background()
	}
	session := GetXormSession(pctx)
	if session.IsInTx() {
		return pctx, &xormCommitter{session: session}, nil
	}
	if err := session.Begin(); err != nil {
		return nil, nil, err
	}
	return newContext(pctx, session), &xormCommitter{session: session}, nil
}

func WithTx(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("nil fn")
	}
	txContext, committer, err := TxContext(ctx)
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

func Context(pctx context.Context) (context.Context, Closer) {
	if pctx == nil {
		pctx = context.Background()
	}
	session := NewXormSession(pctx)
	return newContext(pctx, session), &xormCloser{session: session}
}

func AutoCloseContext(pctx context.Context) context.Context {
	if pctx == nil {
		pctx = context.Background()
	}
	return newContext(pctx, NewAutoCloseXormSession(pctx))
}

func GetXormSession(ctx context.Context) *xorm.Session {
	if ctx != nil {
		if xctx, ok := ctx.(*xormContext); ok {
			return xctx.session
		}
	}
	return NewXormSession(ctx)
}

func NewXormSession(ctx context.Context) *xorm.Session {
	session := engine.NewSession()
	session.Context(ctx)
	return session
}

func NewAutoCloseXormSession(ctx context.Context) *xorm.Session {
	return engine.Context(ctx)
}
