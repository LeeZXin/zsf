package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/LeeZXin/zsf/llm/engine"
	"github.com/LeeZXin/zsf/llm/tool"
)

// fakeAdapter 脚本化适配器（消息类型直接取 string）：按 responses 队列
// 回放补全结果（空队列返回固定文本），并记录每轮收到的 system 消息，
// 供会话级测试校验实际下发的 prompt 与工具循环。
type fakeAdapter struct {
	systems   []string
	responses []engine.CompletionResult
	tools     []tool.Tool
}

func (f *fakeAdapter) DefaultModel() string  { return "m" }
func (f *fakeAdapter) CompressModel() string { return "m" }
func (f *fakeAdapter) DefaultTools() []tool.Tool {
	return f.tools
}
func (f *fakeAdapter) Complete(_ context.Context, messages []string, _ string, _ []tool.Tool) (engine.CompletionResult, error) {
	f.systems = append(f.systems, messages[0])
	if len(f.responses) == 0 {
		return engine.CompletionResult{Content: "ok"}, nil
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return r, nil
}
func (f *fakeAdapter) OpenStream(_ context.Context, _ []string, _ string, _ []tool.Tool) (engine.StreamReader, error) {
	return nil, errors.New("not used")
}
func (f *fakeAdapter) MessageRole(string) string            { return tool.User }
func (f *fakeAdapter) MessageContent(m string) string       { return m }
func (f *fakeAdapter) ConvertToUserMessage(c string) string { return c }
func (f *fakeAdapter) ConvertToSystemMessage(c string) string {
	return c
}
func (f *fakeAdapter) ConvertToToolMessage(_ engine.ToolCall, c string) string {
	return c
}
func (f *fakeAdapter) ConvertToAssistantMessage(res engine.CompletionResult) string {
	return res.Content
}
func (f *fakeAdapter) ClearReasoningContent(ms []string) []string { return ms }

// TestSetExtraPromptFromToolCallback 工具回调（会话持主锁路径）内
// SetExtraPrompt：无锁替换不产生死锁；下一轮组装消息时基础 Prompt +
// 新 ExtraPrompt 拼接下发（基础 prompt 不受影响）。
func TestSetExtraPromptFromToolCallback(t *testing.T) {
	var s *Session[string]
	swap := tool.NewTool[struct{}]("swap_extra", "替换附加 prompt",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			s.SetExtraPrompt("\n\nextra-v2")
			return ctx, "swapped", nil
		})
	adapter := &fakeAdapter{
		responses: []engine.CompletionResult{
			{FinishReason: tool.ToolCalls, ToolCalls: []engine.ToolCall{{ID: "c1", Name: "swap_extra", Arguments: "{}"}}},
			{Content: "done"},
			{Content: "ok"},
		},
		tools: []tool.Tool{swap},
	}
	s = New(adapter, Config[string]{Prompt: "base-v1"})
	if _, err := s.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	// 第 1 轮内两次 Complete（工具调用 + 收尾）都还是旧 prompt——
	// SetExtraPrompt 对下一轮生效；第 2 轮起为 base + 新 extra。
	if got := adapter.systems[0]; got != "base-v1" {
		t.Fatalf("首轮 system prompt = %q", got)
	}
	if got := adapter.systems[len(adapter.systems)-1]; got != "base-v1\n\nextra-v2" {
		t.Fatalf("下一轮 system prompt = %q", got)
	}
}

// TestEffectivePromptFallback 基础 prompt + ExtraPrompt + 摘要的拼接语义。
func TestEffectivePromptFallback(t *testing.T) {
	s := New(&fakeAdapter{}, Config[string]{Prompt: "static"})
	if got := s.effectivePrompt(); got != "static" {
		t.Fatalf("基础 prompt = %q", got)
	}
	s.SetExtraPrompt("\n\nextra")
	if got := s.effectivePrompt(); got != "static\n\nextra" {
		t.Fatalf("基础 + extra = %q", got)
	}
	s.summary = "sum"
	if got := s.effectivePrompt(); got != "static\n\nextra" {
		t.Fatalf("摘要不得拼进 system prompt：%q", got)
	}
}

// recordingAdapter 记录每次 Complete 收到的消息切片（拷贝），
// 供前缀稳定性测试校验跨轮消息结构。
type recordingAdapter struct {
	fakeAdapter
	calls [][]string
}

func (r *recordingAdapter) Complete(_ context.Context, messages []string, _ string, _ []tool.Tool) (engine.CompletionResult, error) {
	r.calls = append(r.calls, slices.Clone(messages))
	return r.fakeAdapter.Complete(context.Background(), messages, "", nil)
}

// TestSystemPrefixStability 锁住 prompt 缓存不变式：全部 API 调用的
// system 消息内容一致、历史仅尾部追加（每次调用的消息列表是上一次
// 调用消息列表的前缀），保证 DeepSeek 自动上下文缓存跨轮命中。
// 未来重构（消息组装/裁剪/清理路径）不得破坏该结构。
func TestSystemPrefixStability(t *testing.T) {
	rec := &recordingAdapter{
		fakeAdapter: fakeAdapter{
			responses: []engine.CompletionResult{
				{FinishReason: tool.ToolCalls, ToolCalls: []engine.ToolCall{{ID: "c1", Name: "noop", Arguments: "{}"}}},
				{Content: "done"},
				{Content: "ok"},
				{Content: "ok"},
			},
		},
	}
	noop := tool.NewTool[struct{}]("noop", "无操作",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return ctx, "noop", nil
		})
	rec.tools = []tool.Tool{noop}
	s := New(rec, Config[string]{Prompt: "base"})
	for i := 0; i < 3; i++ {
		if _, err := s.Send(context.Background(), fmt.Sprintf("u%d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.calls) != 4 {
		t.Fatalf("调用次数 = %d，期望 4（首轮工具循环 2 次 + 两轮各 1 次）", len(rec.calls))
	}
	for i, call := range rec.calls {
		if len(call) == 0 || call[0] != "base" {
			t.Fatalf("第 %d 次调用 system = %q，期望恒为 %q", i, call, "base")
		}
	}
	// 前缀不变式：第 i+1 次调用的消息（除新 user 外）以第 i 次调用为前缀。
	for i := 0; i < len(rec.calls)-1; i++ {
		prev, next := rec.calls[i], rec.calls[i+1]
		if len(next) < len(prev) {
			t.Fatalf("第 %d 次调用消息比前一次短：%d < %d", i+1, len(next), len(prev))
		}
		if !slices.Equal(prev, next[:len(prev)]) {
			t.Fatalf("第 %d 次调用破坏前缀不变式：\nprev=%q\nnext=%q", i+1, prev, next)
		}
	}
}

func TestGetSnapshotRoundTripSummary(t *testing.T) {
	s := New(&fakeAdapter{}, Config[string]{Prompt: "p"})
	s.summary = "compressed-summary"
	s.history = []string{"u1", "a1"}
	snap := s.GetSnapshot()
	if snap.Summary != "compressed-summary" {
		t.Fatalf("GetSnapshot 丢失 Summary: %q", snap.Summary)
	}
	s2 := New(&fakeAdapter{}, Config[string]{Snapshot: &snap})
	if s2.summary != "compressed-summary" {
		t.Fatalf("restore 丢失 Summary: %q", s2.summary)
	}
	if len(s2.history) != 3 {
		t.Fatalf("restore 应补压缩片段，history = %v", s2.history)
	}
	if !strings.Contains(s2.history[0], CompactFragmentOpen) {
		t.Fatalf("restore 首条应为压缩片段：%v", s2.history)
	}
}

type tmsg struct {
	role, content string
}

type recAdapter struct {
	calls     [][]tmsg
	responses []engine.CompletionResult
	tools     []tool.Tool
}

func (a *recAdapter) DefaultModel() string  { return "m" }
func (a *recAdapter) CompressModel() string { return "m" }
func (a *recAdapter) DefaultTools() []tool.Tool {
	return a.tools
}
func (a *recAdapter) Complete(_ context.Context, messages []tmsg, _ string, _ []tool.Tool) (engine.CompletionResult, error) {
	a.calls = append(a.calls, slices.Clone(messages))
	if len(a.responses) == 0 {
		return engine.CompletionResult{Content: "ok"}, nil
	}
	r := a.responses[0]
	a.responses = a.responses[1:]
	return r, nil
}
func (a *recAdapter) OpenStream(context.Context, []tmsg, string, []tool.Tool) (engine.StreamReader, error) {
	return nil, errors.New("not used")
}
func (a *recAdapter) MessageRole(m tmsg) string    { return m.role }
func (a *recAdapter) MessageContent(m tmsg) string { return m.content }
func (a *recAdapter) ConvertToUserMessage(c string) tmsg {
	return tmsg{role: tool.User, content: c}
}
func (a *recAdapter) ConvertToSystemMessage(c string) tmsg {
	return tmsg{role: roleSystem, content: c}
}
func (a *recAdapter) ConvertToToolMessage(_ engine.ToolCall, c string) tmsg {
	return tmsg{role: roleTool, content: c}
}
func (a *recAdapter) ConvertToAssistantMessage(res engine.CompletionResult) tmsg {
	return tmsg{role: roleAssistant, content: res.Content}
}
func (a *recAdapter) ClearReasoningContent(ms []tmsg) []tmsg { return ms }

func TestMidLoopCompactKeepsUserDropsOldToolPair(t *testing.T) {
	echo := tool.NewTool[struct{}]("echo", "d",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return ctx, strings.Repeat("x", 40), nil
		})
	ad := &recAdapter{
		tools: []tool.Tool{echo},
		responses: []engine.CompletionResult{
			{FinishReason: tool.ToolCalls, ToolCalls: []engine.ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}}},
			{FinishReason: tool.ToolCalls, ToolCalls: []engine.ToolCall{{ID: "2", Name: "echo", Arguments: "{}"}}},
			{Content: "done"},
		},
	}
	s := New(ad, Config[tmsg]{
		Prompt:           "sys",
		MaxHistoryTokens: 50,
		TokenCounter:     func(text string) uint64 { return uint64(len(text)) },
		Compress:         func(context.Context, []tmsg) (string, error) { return "SUM", nil },
	})
	if _, err := s.Send(context.Background(), "task", nil); err != nil {
		t.Fatal(err)
	}
	if len(ad.calls) != 3 {
		t.Fatalf("Complete 次数 = %d，期望 3（两轮工具 + 收尾）", len(ad.calls))
	}
	last := ad.calls[len(ad.calls)-1]
	if last[0].role != roleSystem || last[0].content != "sys" {
		t.Fatalf("压缩后 system 应保持原 prompt，got %#v", last[0])
	}
	var users, tools, fragments int
	for _, m := range last[1:] {
		switch m.role {
		case tool.User:
			if strings.Contains(m.content, CompactFragmentOpen) {
				fragments++
				if !strings.Contains(m.content, "SUM") {
					t.Fatalf("压缩片段应含摘要，got %#v", m)
				}
			} else {
				users++
			}
		case roleTool:
			tools++
		}
	}
	if fragments != 1 {
		t.Fatalf("应注入 1 条压缩片段，got %d messages=%v", fragments, last)
	}
	if users != 1 {
		t.Fatalf("当前轮 user 应保留 1 条，got %d messages=%v", users, last)
	}
	if tools != 1 {
		t.Fatalf("应只保留最近一轮 tool 结果，got %d messages=%v", tools, last)
	}
}

type overflowAdapter struct {
	recAdapter
	completeFn func(call int, messages []tmsg) (engine.CompletionResult, error)
	n          int
}

func (a *overflowAdapter) Complete(_ context.Context, messages []tmsg, _ string, _ []tool.Tool) (engine.CompletionResult, error) {
	a.n++
	a.calls = append(a.calls, slices.Clone(messages))
	return a.completeFn(a.n, messages)
}

func TestOverflowRecoverDropsOlderTurn(t *testing.T) {
	ad := &overflowAdapter{
		completeFn: func(n int, messages []tmsg) (engine.CompletionResult, error) {
			if n == 1 {
				return engine.CompletionResult{Content: "first"}, nil
			}
			if n == 2 {
				return engine.CompletionResult{}, errors.New("maximum context length exceeded")
			}
			return engine.CompletionResult{Content: "recovered"}, nil
		},
	}
	s := New(ad, Config[tmsg]{
		Prompt:   "sys",
		Compress: func(context.Context, []tmsg) (string, error) { return "SUM", nil },
	})
	if _, err := s.Send(context.Background(), "u1", nil); err != nil {
		t.Fatal(err)
	}
	resp, err := s.Send(context.Background(), "u2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "recovered" {
		t.Fatalf("resp = %q", resp)
	}
	if ad.n != 3 {
		t.Fatalf("Complete 次数 = %d，期望 3（首轮 1 + 次轮 overflow + 重试）", ad.n)
	}
	last := ad.calls[len(ad.calls)-1]
	for _, m := range last {
		if m.role == tool.User && m.content == "u1" {
			t.Fatalf("overflow 压缩应丢掉旧轮 u1，got %v", last)
		}
	}
}

func TestOverflowRecoveryFailureRollsBackSummary(t *testing.T) {
	ad := &overflowAdapter{
		completeFn: func(n int, _ []tmsg) (engine.CompletionResult, error) {
			if n == 1 {
				return engine.CompletionResult{Content: "first"}, nil
			}
			return engine.CompletionResult{}, errors.New("maximum context length exceeded")
		},
	}
	s := New(ad, Config[tmsg]{
		Prompt:   "sys",
		Compress: func(context.Context, []tmsg) (string, error) { return "SUM", nil },
	})
	if _, err := s.Send(context.Background(), "u1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), "u2", nil); err == nil {
		t.Fatal("重试仍溢出应失败")
	}
	if s.summary != "" {
		t.Fatalf("失败轮摘要应回滚，got %q", s.summary)
	}
	if !strings.Contains(s.effectivePrompt(), "sys") || strings.Contains(s.effectivePrompt(), "SUM") {
		t.Fatalf("失败轮不得把摘要留在 prompt：%q", s.effectivePrompt())
	}
}

func TestInjectFragmentIdleAppendsHistory(t *testing.T) {
	s := New(&fakeAdapter{}, Config[string]{Prompt: "p"})
	s.InjectFragment(InterruptedTurnGuidance)
	if len(s.history) != 1 || !strings.Contains(s.history[0], "turn-aborted") {
		t.Fatalf("空闲 InjectFragment 应写入历史：%v", s.history)
	}
}

func TestGetSnapshotRestoreInjectsCompactFragment(t *testing.T) {
	s := New(&recAdapter{}, Config[tmsg]{Prompt: "p"})
	s.summary = "compressed-summary"
	s.history = []tmsg{{tool.User, "u1"}, {roleAssistant, "a1"}}
	snap := s.GetSnapshot()
	s2 := New(&recAdapter{}, Config[tmsg]{Prompt: "p", Snapshot: &snap})
	if len(s2.history) != 3 {
		t.Fatalf("恢复应补压缩片段，history=%v", s2.history)
	}
	if !strings.Contains(s2.history[0].content, CompactFragmentOpen) || !strings.Contains(s2.history[0].content, "compressed-summary") {
		t.Fatalf("首条应为压缩片段：%v", s2.history[0])
	}
	if s2.effectivePrompt() != "p" {
		t.Fatalf("恢复后 system 不得含摘要：%q", s2.effectivePrompt())
	}
}

func TestClipDoesNotOrphanToolResults(t *testing.T) {
	s := New(&recAdapter{}, Config[tmsg]{
		MaxHistoryTokens: 10,
		TokenCounter:     func(text string) uint64 { return uint64(len(text)) },
		Compress:         func(context.Context, []tmsg) (string, error) { return "", nil },
	})
	body := []tmsg{
		{tool.User, "u"},
		{roleAssistant, "a1"},
		{roleTool, "t1-long-output-here"},
		{roleAssistant, "a2"},
		{roleTool, "t2"},
	}
	out, err := s.trim(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("trim 不应清空")
	}
	if s.adapter.MessageRole(out[0]) == roleTool {
		t.Fatalf("不得以孤儿 tool 结果起头：%v", out)
	}
}
