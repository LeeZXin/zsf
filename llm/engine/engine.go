// Package engine 引擎编排层：经 Adapter 抽象收敛三家厂商 SDK 差异，
// 统一处理工具调用循环、steering 注入、流式合并、用量累积与轮次上限。
// 无状态调用经 RunChatCompletion，有状态对话见 session 包。
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LeeZXin/zsf/llm/event"
	"github.com/LeeZXin/zsf/llm/tool"
	"github.com/LeeZXin/zsf/utils/listutil"
)

// DefaultMaxToolRounds 默认最大工具调用轮次，防止模型陷入工具调用死循环。
const DefaultMaxToolRounds = 16

// ErrMaxToolRounds 工具调用轮次超出上限。
var ErrMaxToolRounds = errors.New("llm: max tool call rounds exceeded")

// Usage token 用量，与具体 SDK 解耦的中性结构（别名，定义于 event 包）。
type Usage = event.Usage

// ToolCall 工具调用，与具体 SDK 解耦的中性结构。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolCallDelta 流式工具调用增量。
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Type      string
	Arguments string
}

// StreamChunk 流式响应块，与具体 SDK 解耦的中性结构。
type StreamChunk struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCallDelta
	FinishReason     string
	Usage            *Usage
	// Raw 保存 SDK 原始响应块，便于需要时取用。
	Raw any
}

// CompletionResult 一轮对话补全的提取结果。
type CompletionResult struct {
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCall
	FinishReason     string
	Usage            Usage
	HasUsage         bool
	// Raw 保存 SDK 原始响应，便于需要时取用。
	Raw any
}

// StreamReader 底层流读取器。
type StreamReader interface {
	Recv() (*StreamChunk, error)
	Close() error
}

// StreamReaderFn 将 recv/close 闭包适配为 StreamReader，
// 避免适配层依赖具体 SDK 的流类型。
type StreamReaderFn struct {
	RecvFn  func() (*StreamChunk, error)
	CloseFn func() error
}

func (r *StreamReaderFn) Recv() (*StreamChunk, error) {
	return r.RecvFn()
}

func (r *StreamReaderFn) Close() error {
	if r.CloseFn == nil {
		return nil
	}
	return r.CloseFn()
}

type Client[M any] interface {
	// DefaultModel 默认加载模型
	DefaultModel() string
	// CompressModel 压缩的模型
	CompressModel() string
	// DefaultTools 默认工具列表
	DefaultTools() []tool.Tool
	// Complete 执行一轮非流式对话补全。
	Complete(ctx context.Context, messages []M, model string, tools []tool.Tool) (CompletionResult, error)
	// OpenStream 开启一轮流式对话补全，返回读取器。
	OpenStream(ctx context.Context, messages []M, model string, tools []tool.Tool) (StreamReader, error)
}

// Adapter 将一个具体的 chat completion SDK 适配到引擎，M 为消息类型。
// 各厂商 SDK 的差异（请求构造、响应解析、消息包装）收敛在 Adapter 实现中，
// 工具调用循环、回调、用量累积、轮次上限等编排逻辑统一由引擎处理。
type Adapter[M any] interface {
	Client[M]
	// MessageRole 返回消息的角色（system/user/assistant/tool），供会话历史裁剪按轮次划分。
	MessageRole(message M) string
	// MessageContent 返回消息的文本内容，供 token 估算与摘要压缩使用。
	MessageContent(message M) string
	// ConvertToUserMessage 转化为 user消息
	ConvertToUserMessage(content string) M
	// ConvertToSystemMessage 转化为 system消息
	ConvertToSystemMessage(content string) M
	// ConvertToToolMessage 转化为tool消息
	ConvertToToolMessage(call ToolCall, content string) M
	// ConvertToAssistantMessage 转化为assistant消息
	ConvertToAssistantMessage(res CompletionResult) M
	// ClearReasoningContent 清空消息中的思考链，返回清理后的切片。
	// 契约：不修改入参及其共享的底层数组/消息对象——会话将本轮完整增量
	// （含思考链）交给 OnMessageAppend 后才调用本方法，清理不得污染
	// 回调方已持有的消息（回调可安全异步消费）。
	// 全部消息无需清空时应原样返回入参（零拷贝）。
	ClearReasoningContent(messages []M) []M
}

// SteerFunc 人机协同转向钩子：每轮工具调用执行完毕后被调用，
// 用于读取用户在工具执行期间插入的新输入。
//
// 返回非空字符串时，该内容作为 user 消息注入下一轮对话，
// 模型据此调整后续行为；返回空字符串表示无新输入，正常继续；
// 返回 error 时中止本次调用。
// 具体输入来源（channel/数据库/消息队列/长轮询等）由上层实现决定，
// channel 场景可直接使用 NewChanSteerFunc。
type SteerFunc func(ctx context.Context) []string

// NewChanSteerFunc 返回基于 channel 的默认 SteerFunc 实现：
// 每轮工具调用后从 ch 读取用户新输入，timeout 内无输入则正常继续下一轮。
//
// 行为约定：
//   - timeout <= 0：非阻塞排空当前已缓冲的消息（timer(0) 会与 chan 随机竞争导致漏读）；
//   - timeout > 0：最多等待 timeout，收到至少一条后把此刻已缓冲的剩余消息一并取出并返回；
//     超时或 ctx 取消时返回已收集到的消息（可能为空），不阻塞模型继续；
//   - ch 被关闭后不再读取（视为输入通道终结）；
//   - 同一 ch 只应供一个对话的 steering 使用，多个对话并发读取会互相争抢消息。
func NewChanSteerFunc(ch <-chan string, timeout time.Duration) SteerFunc {
	return func(ctx context.Context) []string {
		ret := make([]string, 0)
		if timeout <= 0 {
			return drainSteerChan(ctx, ch, ret)
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case input, ok := <-ch:
			if !ok {
				return ret
			}
			ret = append(ret, input)
			return drainSteerChan(ctx, ch, ret)
		case <-ctx.Done():
			return ret
		case <-timer.C:
			return ret
		}
	}
}

// drainSteerChan 非阻塞取出 ch 中已缓冲的全部消息。
func drainSteerChan(ctx context.Context, ch <-chan string, ret []string) []string {
	for {
		select {
		case input, ok := <-ch:
			if !ok {
				return ret
			}
			ret = append(ret, input)
		case <-ctx.Done():
			return ret
		default:
			return ret
		}
	}
}

// RunOptions 引擎运行选项。
type RunOptions[M any] struct {
	// MaxToolRounds 工具调用最大轮次，<=0 时使用 DefaultMaxToolRounds。
	MaxToolRounds int
	// Steer 每轮工具调用执行完毕后读取用户新输入，非空则注入下一轮。
	// channel 场景可通过 NewChanSteerFunc 构建。
	Steer SteerFunc
	// Stream 为 true 时走流式调用：模型输出以 stream_chunk 事件逐块分发
	// （事件 Content 为 *StreamChunk，含增量文本与 SDK 原始块），
	// 经事件接收器实现打字机式推送；false 时走非流式。
	Stream bool
	// Events 结构化事件接收器：一次 RunChatCompletion 产生的所有事件经其分发。
	// 需要多路观察（如会话级审计 + 本轮实时推送）时用 MultiEventSink 组合；
	// Session 路径下会话级与本轮接收器已自动并入（SessionID 自动补全）。
	Events event.Sink
	// 模型名称
	Model string
	// 工具列表
	Tools []tool.Tool
	// CompactContext 每次 LLM 调用前（含首轮，以及工具 + steering 之后）压缩消息。
	// nil 时不压缩、overflow 也不恢复。Session 默认挂上会话 trim。
	CompactContext CompactContextFunc[M]
}

func (o *RunOptions[M]) maxToolRounds() int {
	if o != nil && o.MaxToolRounds > 0 {
		return o.MaxToolRounds
	}
	return DefaultMaxToolRounds
}

// eventDispatcher 统一事件分发入口：ctx 已注入接收器时（Session 执行路径，
// WithEventSink 注入的复合 sink）以 ctx 注入者为准；独立使用引擎时取 opts.Events。
func eventDispatcher[M any](ctx context.Context, opts *RunOptions[M]) event.Sink {
	if sink, ok := event.SinkFromContext(ctx); ok {
		return sink
	}
	if opts == nil {
		return nil
	}
	return opts.Events
}

// RunChatCompletion 编排一次对话补全：
//  1. 每轮 LLM 调用前经 CompactContext（阈值）压缩消息，使长任务的工具循环内也能腾窗口；
//  2. 调用 Complete（或 opts.Stream 时 OpenStream），
//     若响应以 tool_calls 结束则执行工具并追加 tool 消息后继续；
//  3. 若配置了 RunOptions.Steer，每轮工具执行后读取用户新输入注入下一轮（人机协同转向）；
//  4. LLM 调用因上下文溢出失败时，经 CompactContext（overflow）压缩后重试一次；
//  5. 轮次超过上限时返回 ErrMaxToolRounds，防止模型陷入工具调用死循环。
//
// 返回最后一轮的 CompletionResult、完整消息列表以及每轮用量。
func RunChatCompletion[M any](ctx context.Context, adapter Adapter[M], messages []M, opts *RunOptions[M]) (CompletionResult, []M, []Usage, error) {
	if opts == nil {
		opts = new(RunOptions[M])
	}
	toolMap := buildToolMap(opts.Tools)
	events := eventDispatcher(ctx, opts)
	usages := make([]Usage, 0, 4)
	var last CompletionResult
	overflowUsed := false
	for round := 0; round < opts.maxToolRounds(); {
		var err error
		messages, err = applyCompact(ctx, messages, opts, CompactThreshold)
		if err != nil {
			return CompletionResult{}, messages, usages, err
		}
		res, err := completeOnce(ctx, adapter, messages, opts, events)
		if err != nil {
			if ctx.Err() != nil {
				return CompletionResult{}, messages, usages, err
			}
			next, ok := recoverOverflow(ctx, messages, opts, err, overflowUsed)
			if !ok {
				if overflowUsed && IsContextOverflow(err) {
					return CompletionResult{}, messages, usages, fmt.Errorf("%w: %w", ErrOverflowRecovery, err)
				}
				return CompletionResult{}, messages, usages, err
			}
			overflowUsed = true
			messages = next
			continue // 压缩后重试本轮，不消耗工具轮次计数
		}
		last = res
		if res.HasUsage {
			usages = append(usages, res.Usage)
		}
		messages = append(messages, adapter.ConvertToAssistantMessage(res))
		if res.FinishReason != tool.ToolCalls || len(res.ToolCalls) == 0 {
			return last, messages, usages, nil
		}
		ctx, messages = executeToolCalls(ctx, adapter, messages, res.ToolCalls, toolMap, events)
		messages, err = applySteer(ctx, adapter, messages, opts)
		if err != nil {
			return CompletionResult{}, messages, usages, err
		}
		round++
	}
	return last, messages, usages, ErrMaxToolRounds
}

// completeOnce 执行一轮 LLM 调用（流式或非流式）。
func completeOnce[M any](ctx context.Context, adapter Adapter[M], messages []M, opts *RunOptions[M], events event.Sink) (CompletionResult, error) {
	model := opts.Model
	if model == "" {
		model = adapter.DefaultModel()
	}
	if opts.Stream {
		reader, err := adapter.OpenStream(ctx, messages, model, opts.Tools)
		if err != nil {
			return CompletionResult{}, err
		}
		res, err := collectStream(&panicSafeReader{inner: reader}, events)
		_ = reader.Close()
		return res, err
	}
	return adapter.Complete(ctx, messages, model, opts.Tools)
}

// applyCompact 调用 CompactContext；未配置时原样返回。
func applyCompact[M any](ctx context.Context, messages []M, opts *RunOptions[M], reason CompactReason) ([]M, error) {
	if opts == nil || opts.CompactContext == nil {
		return messages, nil
	}
	return opts.CompactContext(ctx, messages, reason)
}

// recoverOverflow 上下文溢出时压缩一次：CompactContext 成功且非空则重试。
// 允许条数不变（例如丢掉一条历史的同时补上摘要 system）；变长视为未压缩。
func recoverOverflow[M any](ctx context.Context, messages []M, opts *RunOptions[M], err error, alreadyUsed bool) ([]M, bool) {
	if alreadyUsed || opts == nil || opts.CompactContext == nil || !IsContextOverflow(err) {
		return nil, false
	}
	next, cErr := opts.CompactContext(ctx, messages, CompactOverflow)
	if cErr != nil || len(next) == 0 || len(next) > len(messages) {
		return nil, false
	}
	return next, true
}

// applySteer 工具执行完毕后调用 Steer 钩子读取用户新输入，
// 非空则作为 user 消息注入消息列表，让模型在下一轮据此调整方向。
func applySteer[M any](ctx context.Context, adapter Adapter[M], messages []M, opts *RunOptions[M]) ([]M, error) {
	if opts == nil || opts.Steer == nil {
		return messages, nil
	}
	input := opts.Steer(ctx)
	if len(input) > 0 {
		messages = append(messages, listutil.MapNe(input, func(t string) M {
			return adapter.ConvertToUserMessage(t)
		})...)
	}
	return messages, nil
}

var errUnknownTool = errors.New("unknown tool")

// ToolCallEvent 一次工具调用的上下文信息，用于组装 tool_call_started /
// tool_call_completed 事件（内部结构，事件经 event.Sink 分发）。
type ToolCallEvent struct {
	ToolCallID string
	Name       string
	Arguments  string
	// 以下字段仅 tool_call_completed 时有效。
	Result   string
	Err      error
	Duration time.Duration
}

// executeToolCalls 并发执行响应中的工具调用：声明可并行的调用并行推进，
// 互斥工具（tool.Exclusive / ParallelAware=false）经 RW 锁串行——
// 并行取读锁、互斥取写锁，避免写文件与 HITL 审批交错。
// 结果按原调用顺序以 tool 消息追加，保证消息列表与模型调用序一致。
// 未匹配到已注册工具的调用会回喂 "unknown tool" 错误，让模型有机会自我纠正。
//
// 并发契约：Invoker 必须并发安全（事件观察器同样——并发路径下事件
// 会交错到达，可经 ToolCallID 关联）；Invoke 返回的 ctx 在并发路径下
// 被忽略——ctx 副作用经共享父 ctx 传递，工具不应依赖返回 ctx 链式
// 传递（当前全部工具实现均原样返回入参 ctx）。
func executeToolCalls[M any](ctx context.Context, adapter Adapter[M], messages []M, toolCalls []ToolCall, toolMap map[string]tool.Tool, events event.Sink) (context.Context, []M) {
	results := make([]M, len(toolCalls))
	var gate sync.RWMutex
	var wg sync.WaitGroup
	for i, toolCall := range toolCalls {
		t, ok := toolMap[toolCall.Name]
		if !ok {
			results[i] = adapter.ConvertToToolMessage(toolCall, fmt.Sprintf("%s: %s", errUnknownTool.Error(), toolCall.Name))
			emitToolCallCompleted(events, &ToolCallEvent{
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Arguments:  toolCall.Arguments,
				Err:        fmt.Errorf("%w: %s", errUnknownTool, toolCall.Name),
			})
			continue
		}
		wg.Add(1)
		go func(i int, toolCall ToolCall, t tool.Tool) {
			defer wg.Done()
			if tool.SupportsParallel(t) {
				gate.RLock()
				defer gate.RUnlock()
			} else {
				gate.Lock()
				defer gate.Unlock()
			}
			ev := &ToolCallEvent{
				ToolCallID: toolCall.ID,
				Name:       toolCall.Name,
				Arguments:  toolCall.Arguments,
			}
			emitToolCallStarted(events, ev)
			begin := time.Now()
			defer func() {
				if p := recover(); p != nil {
					ev.Err = NewPanicErr(p, debug.Stack())
				}
				ev.Duration = time.Since(begin)
				if ev.Err != nil {
					results[i] = adapter.ConvertToToolMessage(toolCall, "error: "+ev.Err.Error())
				} else {
					results[i] = adapter.ConvertToToolMessage(toolCall, ev.Result)
				}
				emitToolCallCompleted(events, ev)
			}()
			_, ev.Result, ev.Err = t.Invoke(ctx, toolCall.ID, toolCall.Name, toolCall.Arguments)
		}(i, toolCall, t)
	}
	wg.Wait()
	messages = append(messages, results...)
	return ctx, messages
}

// emitToolCallStarted / emitToolCallCompleted 将工具调用事件转为结构化事件。
func emitToolCallStarted(events event.Sink, ev *ToolCallEvent) {
	if events == nil {
		return
	}
	events.Emit(event.Event{
		Type:       event.ToolCallStarted,
		Timestamp:  time.Now(),
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.Name,
		Arguments:  ev.Arguments,
	})
}

func emitToolCallCompleted(events event.Sink, ev *ToolCallEvent) {
	if events == nil {
		return
	}
	e := event.Event{
		Type:       event.ToolCallCompleted,
		Timestamp:  time.Now(),
		Duration:   ev.Duration,
		ToolCallID: ev.ToolCallID,
		ToolName:   ev.Name,
		Arguments:  ev.Arguments,
		Content:    ev.Result,
	}
	if ev.Err != nil {
		e.Err = ev.Err.Error()
	}
	events.Emit(e)
}

// collectStream 从读取器收集一轮流式响应，增量合并为 CompletionResult，
// 不缓存全量 chunk，内存占用与响应大小无关。
func collectStream(reader StreamReader, events event.Sink) (CompletionResult, error) {
	var (
		ret     CompletionResult
		content strings.Builder
		reason  strings.Builder
		calls   = make(map[int]*toolCallBuilder)
	)
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ret, err
		}
		content.WriteString(chunk.Content)
		reason.WriteString(chunk.ReasoningContent)
		if chunk.FinishReason != "" {
			ret.FinishReason = chunk.FinishReason
		}
		if chunk.Usage != nil && (!ret.HasUsage || ret.Usage.TotalTokens < chunk.Usage.TotalTokens) {
			ret.Usage = *chunk.Usage
			ret.HasUsage = true
		}
		for _, toolCall := range chunk.ToolCalls {
			b := calls[toolCall.Index]
			if b == nil {
				b = &toolCallBuilder{}
				calls[toolCall.Index] = b
			}
			if toolCall.ID != "" {
				b.id = toolCall.ID
			}
			if toolCall.Name != "" {
				b.name = toolCall.Name
			}
			if toolCall.Type != "" {
				b.typ = toolCall.Type
			}
			b.args.WriteString(toolCall.Arguments)
		}
		if events != nil {
			events.Emit(event.Event{
				Type:            event.StreamChunk,
				Timestamp:       time.Now(),
				ChunkContentLen: len(chunk.Content) + len(chunk.ReasoningContent),
				Content:         chunk,
			})
		}
	}
	ret.Content = content.String()
	ret.ReasoningContent = reason.String()
	ret.ToolCalls = buildToolCalls(calls)
	return ret, nil
}

type toolCallBuilder struct {
	id, name, typ string
	args          strings.Builder
}

func buildToolCalls(calls map[int]*toolCallBuilder) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(calls))
	for idx := range calls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	ret := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		b := calls[idx]
		ret = append(ret, ToolCall{
			ID:        b.id,
			Name:      b.name,
			Arguments: b.args.String(),
		})
	}
	return ret
}

func buildToolMap(tools []tool.Tool) map[string]tool.Tool {
	m := make(map[string]tool.Tool, len(tools))
	for _, t := range tools {
		m[t.GetFunctionName()] = t
	}
	return m
}

// panicSafeReader 防止底层 SDK 读取时 panic 导致调用方崩溃。
type panicSafeReader struct {
	inner StreamReader
}

func (r *panicSafeReader) Recv() (chunk *StreamChunk, err error) {
	defer func() {
		if p := recover(); p != nil {
			chunk, err = nil, NewPanicErr(p, debug.Stack())
		}
	}()
	return r.inner.Recv()
}

func (r *panicSafeReader) Close() error {
	return r.inner.Close()
}

// NewAdapter 由 API key 与请求参数构造对应消息类型 M 的 Adapter。
type NewAdapter[M any] func(string, *Options) Adapter[M]
