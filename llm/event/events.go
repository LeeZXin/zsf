// Package event 结构化事件流：Event / Sink 及多路组合（MultiEventSink），
// 覆盖轮次、工具调用、压缩、steering、HITL 审批与会话生命周期事件。
// 事件双发到会话级观察器（全局审计）与本轮观察器（当前请求实时推送）。
package event

import (
	"context"
	"time"
)

// Usage token 用量，与具体 SDK 解耦的中性结构。
// 作为事件载荷类型（round_completed 事件）定义于本包，
// engine/session 经别名或直接引用复用。
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// Type 结构化事件类型。
type Type string

const (
	// RoundStarted 一轮对话开始执行。
	RoundStarted Type = "round_started"
	// RoundCompleted 一轮对话成功完成。
	RoundCompleted Type = "round_completed"
	// RoundFailed 一轮对话失败（模型调用错误、工具循环错误、被取消等）。
	RoundFailed Type = "round_failed"
	// ToolCallStarted 引擎开始执行一个工具调用。
	ToolCallStarted Type = "tool_call_started"
	// ToolCallCompleted 工具调用执行完毕（成功或失败）。
	ToolCallCompleted Type = "tool_call_completed"
	// ToolProgress 工具执行期间的进度输出（增量，高频事件）：
	// 如 shell 工具的子进程实时输出。Content 为 string 增量块。
	// 纯展示语义：不进对话消息流、不影响模型上下文（模型只收到
	// tool_call_completed 的最终结果），消费方按 ToolCallID 关联到调用。
	ToolProgress Type = "tool_progress"
	// StreamChunk 流式响应收到一个 chunk（高频事件）。
	StreamChunk Type = "stream_chunk"
	// CompressStarted 历史压缩开始（裁剪触发）。
	CompressStarted Type = "compress_started"
	// CompressCompleted 历史压缩完成（或失败）。
	CompressCompleted Type = "compress_completed"
	// SteerInjected 会话执行期间注入了一条 steering 用户消息。
	SteerInjected Type = "steer_injected"
	// SessionClosed 会话被关闭。
	SessionClosed Type = "session_closed"
	// ApprovalRequested 工具请求用户确认（HITL）：经事件流这一独立通道
	// 发出，不嵌入对话消息流。带 ToolName/Arguments 与 Content（审批的可读描述），
	// 业务订阅该事件后应向前端展示"有什么动作需要审批"并等待其决定。
	ApprovalRequested Type = "approval_requested"
)

// Event 结构化事件。字段按事件类型可选填充（空值在 JSON 序列化时省略）。
// 事件由引擎与 Session 产生，经 Sink 分发（典型消费：日志、metrics、审计）。
type Event struct {
	Type      Type      `json:"type"`
	SessionID string    `json:"sessionId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	// Round 事件所属对话轮次（round_* 事件）。
	Round int64 `json:"round,omitempty"`
	// Duration 事件耗时（tool_call_completed / round_* / compress_completed）。
	Duration time.Duration `json:"durationNs,omitempty"`
	// Err 错误信息（round_failed / tool_call_completed 失败 / compress_completed 失败）。
	Err string `json:"error,omitempty"`
	// Usage 本轮用量（round_completed）。
	Usage *Usage `json:"usage,omitempty"`
	// ToolCallID 工具调用 ID（tool_call_* 事件）。
	ToolCallID string `json:"toolCallId,omitempty"`
	// ToolName 工具名（tool_call_* 事件）。
	ToolName string `json:"toolName,omitempty"`
	// Arguments 工具调用参数（tool_call_* 事件）。
	Arguments string `json:"arguments,omitempty"`
	// Content 事件载荷，类型随事件类型而定：
	//   - steer_injected 的用户消息、tool_call_completed 的工具结果、
	//     compress_completed 的摘要、approval_requested 的审批可读描述、
	//     tool_progress 的进度增量块：string；
	//   - stream_chunk：*StreamChunk（含增量文本与 SDK 原始块）。
	Content any `json:"content,omitempty"`
	// DroppedCount / KeptCount 压缩裁剪/保留的消息数（compress_started）。
	DroppedCount int `json:"droppedCount,omitempty"`
	KeptCount    int `json:"keptCount,omitempty"`
	// ChunkContentLen 流式 chunk 的文本长度（stream_chunk，免反序列化 Content 的便利字段）。
	ChunkContentLen int `json:"chunkContentLen,omitempty"`
}

// Sink 结构化事件接收器。
// 实现必须并发安全：流式 chunk 与并行工具（如 parallel）会并发触发 Emit。
type Sink interface {
	Emit(event Event)
}

// SinkFunc 将函数适配为 Sink。
type SinkFunc func(event Event)

// Emit 实现 Sink。
func (f SinkFunc) Emit(event Event) {
	f(event)
}

// MultiSink 将多个事件接收器组合为一个（类似 io.MultiWriter）：
// Emit 依次分发到所有非 nil 接收器；全部为 nil 时返回 nil，
// 仅一个非 nil 时原样返回（零包装开销）。
// 便于把会话级、本轮等观察器组合进同一分发点，新增观察器无需改动分发逻辑。
func MultiSink(sinks ...Sink) Sink {
	kept := make([]Sink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			kept = append(kept, s)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	default:
		return multiSink(kept)
	}
}

type multiSink []Sink

// Emit 实现 Sink。
func (m multiSink) Emit(event Event) {
	for _, s := range m {
		s.Emit(event)
	}
}

type sinkContextKey struct{}

// WithSink 将事件接收器注入 ctx（Session 每轮执行时自动注入），
// 供工具在等待用户确认前发出 ApprovalRequested 等事件。
func WithSink(ctx context.Context, sink Sink) context.Context {
	return context.WithValue(ctx, sinkContextKey{}, sink)
}

// SinkFromContext 从 ctx 取事件接收器（Session 执行路径内可用）。
func SinkFromContext(ctx context.Context) (Sink, bool) {
	sink, ok := ctx.Value(sinkContextKey{}).(Sink)
	return sink, ok && sink != nil
}
