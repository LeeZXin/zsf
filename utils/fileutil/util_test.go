package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ok, err := FileExists(p)
	if err != nil || !ok {
		t.Fatalf("存在文件应 (true,nil), got %v %v", ok, err)
	}
	ok, err = FileExists(filepath.Join(dir, "missing"))
	if err != nil || ok {
		t.Fatalf("不存在应 (false,nil), got %v %v", ok, err)
	}
}
