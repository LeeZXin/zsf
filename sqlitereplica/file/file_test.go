package file

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/LeeZXin/zsf/sqlitereplica"
)

func TestOutputAtomic(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir)

	if err := h.Output(context.Background(), "0000000000000001/wal/00000000_00000000.wal.lz4", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}

	// 目标文件存在且内容正确。
	buf, err := os.ReadFile(filepath.Join(dir, "0000000000000001", "wal", "00000000_00000000.wal.lz4"))
	if err != nil {
		t.Fatal(err)
	} else if string(buf) != "hello" {
		t.Fatalf("content = %q, want hello", buf)
	}

	// 无 tmp 残留。
	entries, err := os.ReadDir(filepath.Join(dir, "0000000000000001", "wal"))
	if err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 {
		t.Fatalf("files = %d, want 1 (no tmp leftover)", len(entries))
	}

	// 覆盖写。
	if err := h.Output(context.Background(), "0000000000000001/wal/00000000_00000000.wal.lz4", bytes.NewReader([]byte("world"))); err != nil {
		t.Fatal(err)
	}
	buf, _ = os.ReadFile(filepath.Join(dir, "0000000000000001", "wal", "00000000_00000000.wal.lz4"))
	if string(buf) != "world" {
		t.Fatalf("content after overwrite = %q, want world", buf)
	}
}

func TestOutputInvalidName(t *testing.T) {
	h := NewHandler(t.TempDir())
	if err := h.Output(context.Background(), "../evil", bytes.NewReader(nil)); err == nil {
		t.Fatal("expected error for path traversal name")
	}
}

func TestOutputWriteFailureCleansTmp(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir)

	// 读侧返回错误 → Output 失败且不留下 tmp 文件。
	err := h.Output(context.Background(), "0000000000000001/wal/x.wal.lz4", errReader{})
	if err == nil {
		t.Fatal("expected error")
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "0000000000000001", "wal")); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("files = %d, want 0", len(entries))
	}
}

func TestRestoreLatestGeneration(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir)

	// 两个代，各一个文件。
	for _, gen := range []string{"0000000000000001", "0000000000000003"} {
		if err := h.Output(context.Background(), gen+"/snapshots/00000000.snapshot.lz4", bytes.NewReader([]byte(gen))); err != nil {
			t.Fatal(err)
		}
	}

	dst := t.TempDir()
	gen, err := h.Restore(context.Background(), "", dst)
	if err != nil {
		t.Fatal(err)
	} else if gen != "0000000000000003" {
		t.Fatalf("generation = %q, want 0000000000000003", gen)
	}

	buf, err := os.ReadFile(filepath.Join(dst, gen, "snapshots", "00000000.snapshot.lz4"))
	if err != nil {
		t.Fatal(err)
	} else if string(buf) != gen {
		t.Fatalf("content = %q, want %q", buf, gen)
	}
}

func TestRestoreSpecificGeneration(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir)

	if err := h.Output(context.Background(), "0000000000000001/wal/00000000_00000000.wal.lz4", bytes.NewReader([]byte("seg"))); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	gen, err := h.Restore(context.Background(), "0000000000000001", dst)
	if err != nil {
		t.Fatal(err)
	} else if gen != "0000000000000001" {
		t.Fatalf("generation = %q", gen)
	}

	buf, err := os.ReadFile(filepath.Join(dst, gen, "wal", "00000000_00000000.wal.lz4"))
	if err != nil {
		t.Fatal(err)
	} else if string(buf) != "seg" {
		t.Fatalf("content = %q, want seg", buf)
	}
}

func TestRestoreEmpty(t *testing.T) {
	h := NewHandler(t.TempDir())
	_, err := h.Restore(context.Background(), "", t.TempDir())
	if !errors.Is(err, sqlitereplica.ErrNoGeneration) {
		t.Fatalf("expected ErrNoGeneration, got %v", err)
	}
}

func TestRemoveIdempotent(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(dir)

	if err := h.Output(context.Background(), "0000000000000001/snapshots/00000000.snapshot.lz4", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	if err := h.Remove(context.Background(), "0000000000000001"); err != nil {
		t.Fatal(err)
	}
	// 幂等。
	if err := h.Remove(context.Background(), "0000000000000001"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "0000000000000001")); !os.IsNotExist(err) {
		t.Fatalf("generation dir still exists: %v", err)
	}
}

// errReader 读取时返回错误。
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("injected") }

var _ io.Reader = errReader{}
