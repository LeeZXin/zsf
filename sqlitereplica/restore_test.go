package sqlitereplica

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pierrec/lz4/v4"
)

/*
TestRestoreSnapshotOnly 远端只有快照没有 WAL 段（快照后、首段上传前崩溃的
窗口）时，恢复走 snapshot-only 直达路径。
*/
func TestRestoreSnapshotOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	replicaDir := filepath.Join(dir, "replica")

	// 建库写数据。
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT);`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := db.Exec(`INSERT INTO t (v) VALUES ('snapshot-only');`); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// 手工构造远端布局：仅快照、无 wal 目录。
	genDir := filepath.Join(replicaDir, "0000000000000001", "snapshots")
	if err := os.MkdirAll(genDir, 0700); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(genDir, "00000000.snapshot.lz4"))
	if err != nil {
		t.Fatal(err)
	}
	zw := lz4.NewWriter(f)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	} else if err := zw.Close(); err != nil {
		t.Fatal(err)
	} else if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	restoredPath := filepath.Join(dir, "restored.db")
	if err := Restore(context.Background(), newTestHandler(replicaDir), RestoreOptions{OutputPath: restoredPath}); err != nil {
		t.Fatal(err)
	}

	if got := countRows(t, restoredPath); got != 5 {
		t.Fatalf("restored rows = %d, want 5", got)
	}
}

/*
TestRestoreOutputPathExists 恢复目标已存在时报错。
*/
func TestRestoreOutputPathExists(t *testing.T) {
	dir := t.TempDir()
	replicaDir := filepath.Join(dir, "replica")
	existing := filepath.Join(dir, "existing.db")

	if err := os.WriteFile(existing, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Restore(context.Background(), newTestHandler(replicaDir), RestoreOptions{OutputPath: existing}); err == nil {
		t.Fatal("expected error for existing output path")
	}
}

/*
TestRestoreNoData 远端无数据时返回 ErrNoGeneration。
*/
func TestRestoreNoData(t *testing.T) {
	dir := t.TempDir()
	replicaDir := filepath.Join(dir, "replica")

	if err := Restore(context.Background(), newTestHandler(replicaDir), RestoreOptions{OutputPath: filepath.Join(dir, "out.db")}); !errors.Is(err, ErrNoGeneration) {
		t.Fatalf("expected ErrNoGeneration, got %v", err)
	}
}

/*
TestNonSQLiteFileWaitsThenSyncs path 指向非 sqlite 文件（如 mp3 占位）时组件
静默等待（Sync 不报错、pos 不推进），文件换成合法 sqlite 库后自动开始同步。
*/
func TestNonSQLiteFileWaitsThenSyncs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// 非 sqlite 文件（无 SQLite magic）。
	if err := os.WriteFile(dbPath, []byte("ID3\x03\x00fake mp3 data, not a sqlite database"), 0600); err != nil {
		t.Fatal(err)
	}

	r := newReplica(testOptions(dbPath, newTestHandler(filepath.Join(dir, "replica")), filepath.Join(dir, "meta")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.Run() }()

	// 等待阶段：Sync 静默返回、pos 保持零值。
	time.Sleep(200 * time.Millisecond)
	if !replicaPos(r).IsZero() {
		t.Fatalf("pos should be zero for non-sqlite file, got %s", replicaPos(r).String())
	}
	if err := r.Sync(ctx); err != nil {
		t.Fatalf("sync on non-sqlite file should wait silently, got %v", err)
	}

	// 替换为真正的 sqlite 库后自动开始同步。
	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	db := newTestDB(t, dbPath)
	insertN(t, db, 5)

	waitFor(t, 10*time.Second, func() bool { return !replicaPos(r).IsZero() })
	r.Close()
}
