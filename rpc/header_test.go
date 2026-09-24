package rpc

import (
	"context"
	"testing"
)

func TestGetTraceIdStableWhenHeaderInjected(t *testing.T) {
	ctx := AddHeader(context.Background(), Header{})
	a := GetTraceId(ctx)
	b := GetTraceId(ctx)
	if a == "" || a != b {
		t.Fatalf("同一 ctx 连续读取应稳定，got %q %q", a, b)
	}
	if GetHeader(ctx).Get(TraceId) != a {
		t.Fatal("生成的 traceId 应写回 Header")
	}
}
