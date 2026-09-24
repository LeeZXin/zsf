package engine

import (
	"context"
	"errors"
	"strings"
)

// ErrCannotCompact 无法再压缩上下文（已无安全切点可丢）：overflow 恢复应停止重试。
var ErrCannotCompact = errors.New("llm: context cannot be compacted further")

// ErrOverflowRecovery 上下文溢出压缩后重试仍失败（或无法压缩）。
var ErrOverflowRecovery = errors.New("llm: context overflow recovery failed")

// CompactReason 压缩触发原因：阈值（工具循环内主动腾窗口）或 overflow 恢复。
type CompactReason string

const (
	// CompactThreshold 工具执行后、下一次 LLM 调用前：超 MaxHistory* 时压缩。
	CompactThreshold CompactReason = "threshold"
	// CompactOverflow API 报上下文溢出后：即使未超阈值也须尽量腾出空间，随后重试一次。
	CompactOverflow CompactReason = "overflow"
)

// CompactContextFunc 压缩当前消息列表。返回的切片替换引擎持有的 messages。
// reason 为 Overflow 时即使未超配置阈值也须尽量丢弃旧消息；无法丢弃时返回 ErrCannotCompact。
type CompactContextFunc[M any] func(ctx context.Context, messages []M, reason CompactReason) ([]M, error)

// 非溢出错误（限流等）即使含 token 字样也不当 overflow。
var nonOverflowNeedles = []string{
	"rate limit",
	"too many requests",
	"throttling",
}

// overflowNeedles 各厂商上下文溢出错误的特征子串（小写匹配）。
var overflowNeedles = []string{
	"prompt is too long",
	"request_too_large",
	"exceeds the context window",
	"maximum context length",
	"maximum prompt length",
	"reduce the length of the messages",
	"maximum allowed input length",
	"context_length_exceeded",
	"context length exceeded",
	"exceeds the available context size",
	"greater than the context length",
	"context window exceeds",
	"exceeded model token limit",
	"token limit exceeded",
	"too many tokens",
	"input token count",
	"prompt too long",
	"range of input length should be",
	"please reduce the length",
}

// IsContextOverflow 判定错误是否为上下文窗口溢出（可经压缩后重试）。
// 取消/超时不当溢出；限流等即使含 token 字样也排除。
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, n := range nonOverflowNeedles {
		if strings.Contains(text, n) {
			return false
		}
	}
	for _, n := range overflowNeedles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}
