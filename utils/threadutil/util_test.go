package threadutil

import (
	"strings"
	"testing"
)

func TestRunSafeReturnsNil(t *testing.T) {
	if err := RunSafe(func() {}); err != nil {
		t.Fatal(err)
	}
}

func TestRunSafeRecoversPanic(t *testing.T) {
	cleaned := false
	err := RunSafe(func() { panic("boom") }, func() { cleaned = true })
	if err == nil {
		t.Fatal("panic 应转为 error")
	}
	if !cleaned {
		t.Fatal("cleanup 应执行")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error 应包含 panic 信息: %v", err)
	}
}

func TestRunSafeUnknownPanic(t *testing.T) {
	err := RunSafe(func() { panic(123) })
	if err == nil || !strings.Contains(err.Error(), "unknown errors") {
		t.Fatalf("非 error/string panic 应包装, got %v", err)
	}
}
