package deepseek_client

import (
	"errors"
	"testing"
	"time"

	"github.com/LeeZXin/zsf/llm/engine"
	"github.com/cohesion-org/deepseek-go"
)

// TestRetryDelay 429/5xx 指数退避重试判定：仅可重试状态码进入退避，
// 其余错误（4xx、非 APIError、超次数、禁用）不重试。
func TestRetryDelay(t *testing.T) {
	opts := &engine.Options{RetryMax: 2, RetryBaseDelay: time.Second}
	if d, ok := retryDelay(&deepseek.APIError{StatusCode: 429}, opts, 0); !ok || d != time.Second {
		t.Fatalf("429 首次应退避 1s，got %v/%v", d, ok)
	}
	if d, ok := retryDelay(&deepseek.APIError{StatusCode: 503}, opts, 1); !ok || d != 2*time.Second {
		t.Fatalf("503 二次应退避 2s，got %v/%v", d, ok)
	}
	for _, code := range []int{500, 502, 504} {
		if _, ok := retryDelay(&deepseek.APIError{StatusCode: code}, opts, 0); !ok {
			t.Fatalf("HTTP %d 应可重试", code)
		}
	}
	if _, ok := retryDelay(&deepseek.APIError{StatusCode: 401}, opts, 0); ok {
		t.Fatal("401 不得重试")
	}
	if _, ok := retryDelay(errors.New("net error"), opts, 0); ok {
		t.Fatal("非 APIError 不得重试")
	}
	if _, ok := retryDelay(&deepseek.APIError{StatusCode: 429}, opts, 2); ok {
		t.Fatal("超过 RetryMax 不得重试")
	}
	if _, ok := retryDelay(&deepseek.APIError{StatusCode: 429}, &engine.Options{RetryMax: -1}, 0); ok {
		t.Fatal("RetryMax 负数应禁用重试")
	}
	if _, ok := retryDelay(nil, opts, 0); ok {
		t.Fatal("nil 错误不得重试")
	}
}
