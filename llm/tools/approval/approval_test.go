package approval

import (
	"context"
	"testing"

	"github.com/LeeZXin/zsf/llm/hitl"
	"github.com/LeeZXin/zsf/llm/tool"
)

func TestWithApprovalApproveAlwaysInvokesInner(t *testing.T) {
	inner := tool.NewTool[struct{}]("rm", "rm",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return ctx, "ran", nil
		})
	wrapped := WithApproval(inner, nil, nil)
	ch := make(chan string, 1)
	ch <- "always:"
	ctx := hitl.WithSteerChan(context.Background(), ch)
	_, result, err := wrapped.Invoke(ctx, "id", "rm", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if result != "ran" {
		t.Fatalf("approve_always 应执行原工具，got %q", result)
	}
}

func TestWithApprovalRejectDoesNotInvokeInner(t *testing.T) {
	called := false
	inner := tool.NewTool[struct{}]("rm", "rm",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			called = true
			return ctx, "ran", nil
		})
	wrapped := WithApproval(inner, nil, nil)
	ch := make(chan string, 1)
	ch <- "reject:不要"
	ctx := hitl.WithSteerChan(context.Background(), ch)
	_, result, err := wrapped.Invoke(ctx, "id", "rm", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("拒绝时不应执行原工具")
	}
	if result != "用户拒绝执行：不要" {
		t.Fatalf("got %q", result)
	}
}
