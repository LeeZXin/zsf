// Package errs API 错误识别工具：从三家厂商 SDK 的错误中提取 HTTP 状态码
// （APIStatus）并判定可重试性（IsRetryable），供业务方实现退避重试与降级。
package errs

import (
	"errors"

	"github.com/cohesion-org/deepseek-go"
)

// APIStatus 从模型 API 返回的错误中提取 HTTP 状态码（429/503 等）。
// 适配三家厂商的 SDK 错误类型；非 API 错误（网络错误、上下文取消等）返回 false。
//
// 用法：
//
//	resp, err := session.Send(ctx, "hi")
//	if status, ok := components.APIStatus(err); ok && status == 429 { ... }
func APIStatus(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	// deepseek 的 APIError 为值类型错误（Error 为值接收者）。
	if dsErr, ok := errors.AsType[deepseek.APIError](err); ok {
		return dsErr.StatusCode, true
	}
	return 0, false
}

// IsRetryable 返回 API 错误是否值得重试：
// 429（限流）、500、502、503、504 及 DeepSeek 的 402 余额不足（等待充值后可重试）。
func IsRetryable(err error) bool {
	status, ok := APIStatus(err)
	if !ok {
		return false
	}
	switch status {
	case 402, 429, 500, 502, 503, 504:
		return true
	}
	return false
}
