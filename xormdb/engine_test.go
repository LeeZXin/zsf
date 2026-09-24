package xormdb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type sampleRow struct {
	Id int64  `xorm:"pk autoincr"`
	V  string `xorm:"varchar(32)"`
}

func (sampleRow) TableName() string { return "sample_row" }

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(Config{
		Driver:     Sqlite3,
		Datasource: filepath.Join(t.TempDir(), "t.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	if err := e.GetEngine().Sync(new(sampleRow)); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestGetSessionSurvivesContextWrap(t *testing.T) {
	e := newTestEngine(t)
	ctx, closer := e.Context(context.Background())
	defer closer.Close()
	s1 := e.GetSession(ctx)
	ctx2, cancel := context.WithCancel(context.WithValue(ctx, "k", "v"))
	defer cancel()
	s2 := e.GetSession(ctx2)
	if s1 != s2 {
		t.Fatal("包装后的 context 应取到同一 session")
	}
	if MustGetSession(ctx2) != s1 {
		t.Fatal("MustGetSession 在包装 ctx 上应成功")
	}
}

func TestNestedWithTxDoesNotCommitOuter(t *testing.T) {
	e := newTestEngine(t)
	err := e.WithTx(context.Background(), func(ctx context.Context) error {
		if _, err := e.GetSession(ctx).Insert(&sampleRow{V: "a"}); err != nil {
			return err
		}
		if err := e.WithTx(ctx, func(ctx context.Context) error {
			_, err := e.GetSession(ctx).Insert(&sampleRow{V: "b"})
			return err
		}); err != nil {
			return err
		}
		return errors.New("outer fail")
	})
	if err == nil || err.Error() != "outer fail" {
		t.Fatalf("got %v", err)
	}
	n, err := e.GetEngine().Count(new(sampleRow))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("外层失败应回滚内层插入，count=%d", n)
	}
}

func TestNestedWithTxCommitTogether(t *testing.T) {
	e := newTestEngine(t)
	err := e.WithTx(context.Background(), func(ctx context.Context) error {
		if _, err := e.GetSession(ctx).Insert(&sampleRow{V: "a"}); err != nil {
			return err
		}
		return e.WithTx(ctx, func(ctx context.Context) error {
			_, err := e.GetSession(ctx).Insert(&sampleRow{V: "b"})
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := e.GetEngine().Count(new(sampleRow))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("内外层应一并提交，count=%d", n)
	}
}
