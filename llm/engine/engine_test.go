package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LeeZXin/zsf/llm/event"
	"github.com/LeeZXin/zsf/llm/tool"
)

// strAdapter 脚本化适配器（消息类型直接取 string），
// 仅用于 executeToolCalls 的并发与结果顺序测试。
type strAdapter struct{}

func (strAdapter) DefaultModel() string                   { return "m" }
func (strAdapter) CompressModel() string                  { return "m" }
func (strAdapter) DefaultTools() []tool.Tool              { return nil }
func (strAdapter) MessageRole(string) string              { return "user" }
func (strAdapter) MessageContent(m string) string         { return m }
func (strAdapter) ConvertToUserMessage(c string) string   { return c }
func (strAdapter) ConvertToSystemMessage(c string) string { return c }
func (strAdapter) ConvertToToolMessage(call ToolCall, content string) string {
	return call.Name + ":" + content
}
func (strAdapter) ConvertToAssistantMessage(res CompletionResult) string { return res.Content }
func (strAdapter) ClearReasoningContent(msgs []string) []string          { return msgs }

func (strAdapter) Complete(context.Context, []string, string, []tool.Tool) (CompletionResult, error) {
	panic("not used")
}
func (strAdapter) OpenStream(context.Context, []string, string, []tool.Tool) (StreamReader, error) {
	panic("not used")
}

// TestExecuteToolCallsParallel 验证工具调用并发执行：
// 两个工具都在等 release 信号，若为顺序执行，第二个工具的 started
// 信号永远不会在 release 前到达（测试以超时守卫失败）。
func TestExecuteToolCallsParallel(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	newGated := func(name string) tool.Tool {
		return tool.NewTool[struct{}](name, "desc",
			func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
				started <- name
				<-release
				return nil, name + "-ok", nil
			})
	}
	calls := []ToolCall{
		{ID: "1", Name: "a", Arguments: "{}"},
		{ID: "2", Name: "b", Arguments: "{}"},
	}
	toolMap := buildToolMap([]tool.Tool{newGated("a"), newGated("b")})

	done := make(chan []string, 1)
	go func() {
		_, msgs := executeToolCalls(context.Background(), strAdapter{}, nil, calls, toolMap, nil)
		done <- msgs
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("工具调用疑似顺序执行：release 前第二个调用未启动")
		}
	}
	close(release)
	select {
	case msgs := <-done:
		got := strings.Join(msgs, "|")
		if want := "a:a-ok|b:b-ok"; got != want {
			t.Fatalf("结果顺序错误：got %q want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("工具执行未在 release 后完成")
	}
}

// TestExecuteToolCallsConcurrentInvocation 用 8 个 sleep 工具验证
// 全量并发（若顺序执行总耗时约为 sum，并发约为 max）。
func TestExecuteToolCallsConcurrentInvocation(t *testing.T) {
	const n = 8
	calls := make([]ToolCall, 0, n)
	ts := make([]tool.Tool, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("t%d", i)
		calls = append(calls, ToolCall{ID: name, Name: name, Arguments: "{}"})
		ts = append(ts, tool.NewTool[struct{}](name, "desc",
			func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
				time.Sleep(100 * time.Millisecond)
				return nil, name + "-ok", nil
			}))
	}
	begin := time.Now()
	_, msgs := executeToolCalls(context.Background(), strAdapter{}, nil, calls, buildToolMap(ts), nil)
	elapsed := time.Since(begin)
	if len(msgs) != n {
		t.Fatalf("结果条数错误：got %d want %d", len(msgs), n)
	}
	// 并发应显著快于串行（8*100ms=800ms），放宽到 600ms 防偶发。
	if elapsed > 600*time.Millisecond {
		t.Fatalf("疑似顺序执行：%v", elapsed)
	}
}

func TestExecuteToolCallsExclusiveSerializes(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	newEx := func(name string) tool.Tool {
		return tool.Exclusive(tool.NewTool[struct{}](name, "desc",
			func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
				started <- name
				<-release
				return nil, name + "-ok", nil
			}))
	}
	calls := []ToolCall{
		{ID: "1", Name: "a", Arguments: "{}"},
		{ID: "2", Name: "b", Arguments: "{}"},
	}
	toolMap := buildToolMap([]tool.Tool{newEx("a"), newEx("b")})
	done := make(chan struct{})
	go func() {
		executeToolCalls(context.Background(), strAdapter{}, nil, calls, toolMap, nil)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("互斥工具未启动")
	}
	select {
	case name := <-started:
		t.Fatalf("互斥工具在 release 前启动了第二个：%s", name)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("第二个互斥工具未在 release 后启动")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("互斥工具未完成")
	}
}

// TestExecuteToolCallsOrdering 验证完成顺序与结果顺序解耦：
// 后启动的工具先完成，结果仍按模型调用顺序追加。
func TestExecuteToolCallsOrdering(t *testing.T) {
	newDelay := func(name string, delay time.Duration) tool.Tool {
		return tool.NewTool[struct{}](name, "desc",
			func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
				time.Sleep(delay)
				return nil, name + "-ok", nil
			})
	}
	calls := []ToolCall{
		{ID: "1", Name: "slow", Arguments: "{}"},
		{ID: "2", Name: "fast", Arguments: "{}"},
		{ID: "3", Name: "missing", Arguments: "{}"},
		{ID: "4", Name: "boom", Arguments: "{}"},
	}
	boom := tool.NewTool[struct{}]("boom", "desc",
		func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return nil, "", errors.New("炸了")
		})
	ts := []tool.Tool{newDelay("slow", 150*time.Millisecond), newDelay("fast", 10*time.Millisecond), boom}
	_, msgs := executeToolCalls(context.Background(), strAdapter{}, nil, calls, buildToolMap(ts), nil)
	got := strings.Join(msgs, "|")
	want := "slow:slow-ok|fast:fast-ok|missing:unknown tool: missing|boom:error: 炸了"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

// TestExecuteToolCallsEvents 验证事件并发分发安全（sink 需并发安全，
// 由 Session 契约保证；此处统计数量与 ID 完整性）。
func TestExecuteToolCallsEvents(t *testing.T) {
	var mu sync.Mutex
	var startedIDs, completedIDs []string
	sink := event.SinkFunc(func(ev event.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch ev.Type {
		case event.ToolCallStarted:
			startedIDs = append(startedIDs, ev.ToolCallID)
		case event.ToolCallCompleted:
			completedIDs = append(completedIDs, ev.ToolCallID)
		}
	})
	calls := []ToolCall{
		{ID: "1", Name: "a", Arguments: "{}"},
		{ID: "2", Name: "b", Arguments: "{}"},
	}
	newT := func(name string) tool.Tool {
		return tool.NewTool[struct{}](name, "desc",
			func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, name, nil
			})
	}
	_, msgs := executeToolCalls(context.Background(), strAdapter{}, nil, calls,
		buildToolMap([]tool.Tool{newT("a"), newT("b")}), sink)
	if len(msgs) != 2 {
		t.Fatalf("结果条数错误：%d", len(msgs))
	}
	if len(startedIDs) != 2 || len(completedIDs) != 2 {
		t.Fatalf("事件不完整：started=%v completed=%v", startedIDs, completedIDs)
	}
}

func TestNewChanSteerFuncTimeoutWaits(t *testing.T) {
	ch := make(chan string, 1)
	go func() {
		time.Sleep(40 * time.Millisecond)
		ch <- "hi"
	}()
	got := NewChanSteerFunc(ch, 200*time.Millisecond)(context.Background())
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("timeout 分支应收待到消息，got %v", got)
	}
}

func TestNewChanSteerFuncTimeoutEmpty(t *testing.T) {
	ch := make(chan string)
	start := time.Now()
	got := NewChanSteerFunc(ch, 50*time.Millisecond)(context.Background())
	if len(got) != 0 {
		t.Fatalf("超时应收空，got %v", got)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("timeout 分支未等待")
	}
}

func TestExecuteToolCallsRecoversPanic(t *testing.T) {
	boom := tool.NewTool[struct{}]("boom", "desc",
		func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			panic("炸了")
		})
	ok := tool.NewTool[struct{}]("ok", "desc",
		func(_ context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return nil, "ok-ok", nil
		})
	calls := []ToolCall{
		{ID: "1", Name: "boom", Arguments: "{}"},
		{ID: "2", Name: "ok", Arguments: "{}"},
	}
	_, msgs := executeToolCalls(context.Background(), strAdapter{}, nil, calls, buildToolMap([]tool.Tool{boom, ok}), nil)
	if len(msgs) != 2 {
		t.Fatalf("结果条数 %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "error:") || !strings.Contains(msgs[0], "炸了") {
		t.Fatalf("panic 应转为 tool 错误消息，got %q", msgs[0])
	}
	if msgs[1] != "ok:ok-ok" {
		t.Fatalf("另一工具应正常完成，got %q", msgs[1])
	}
}

type scriptAdapter struct {
	strAdapter
	fn func(call int, messages []string) (CompletionResult, error)
	n  int
}

func (a *scriptAdapter) Complete(_ context.Context, messages []string, _ string, _ []tool.Tool) (CompletionResult, error) {
	a.n++
	return a.fn(a.n, messages)
}

func TestOverflowRecoverOnce(t *testing.T) {
	ad := &scriptAdapter{fn: func(n int, messages []string) (CompletionResult, error) {
		if n == 1 {
			return CompletionResult{}, errors.New("maximum context length exceeded")
		}
		return CompletionResult{Content: "ok"}, nil
	}}
	var overflowCompact int
	opts := &RunOptions[string]{
		CompactContext: func(_ context.Context, messages []string, reason CompactReason) ([]string, error) {
			if reason == CompactOverflow {
				overflowCompact++
				if len(messages) < 2 {
					return messages, ErrCannotCompact
				}
				return messages[1:], nil
			}
			return messages, nil
		},
	}
	_, out, _, err := RunChatCompletion(context.Background(), ad, []string{"sys", "u1", "a1", "u2"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if ad.n != 2 {
		t.Fatalf("Complete 次数 = %d，期望 overflow 后重试 1 次", ad.n)
	}
	if overflowCompact != 1 {
		t.Fatalf("overflow 压缩次数 = %d", overflowCompact)
	}
	if out[len(out)-1] != "ok" {
		t.Fatalf("out = %v", out)
	}
}

func TestOverflowCannotCompactReturnsOriginal(t *testing.T) {
	orig := errors.New("maximum context length exceeded")
	ad := &scriptAdapter{fn: func(int, []string) (CompletionResult, error) {
		return CompletionResult{}, orig
	}}
	_, _, _, err := RunChatCompletion(context.Background(), ad, []string{"u"}, &RunOptions[string]{
		CompactContext: func(_ context.Context, messages []string, reason CompactReason) ([]string, error) {
			if reason == CompactOverflow {
				return messages, ErrCannotCompact
			}
			return messages, nil
		},
	})
	if err == nil || !IsContextOverflow(err) {
		t.Fatalf("无法压缩时应返回原 overflow，got %v", err)
	}
	if errors.Is(err, ErrOverflowRecovery) {
		t.Fatal("尚未重试成功过，不应包 ErrOverflowRecovery")
	}
}

func TestOverflowRetryStillFails(t *testing.T) {
	ad := &scriptAdapter{fn: func(int, []string) (CompletionResult, error) {
		return CompletionResult{}, errors.New("maximum context length exceeded")
	}}
	_, _, _, err := RunChatCompletion(context.Background(), ad, []string{"sys", "u1", "a1", "u2"}, &RunOptions[string]{
		CompactContext: func(_ context.Context, messages []string, reason CompactReason) ([]string, error) {
			if reason == CompactOverflow && len(messages) > 1 {
				return messages[1:], nil
			}
			return messages, nil
		},
	})
	if !errors.Is(err, ErrOverflowRecovery) {
		t.Fatalf("重试仍溢出应包 ErrOverflowRecovery，got %v", err)
	}
}

func TestThresholdCompactBetweenToolRounds(t *testing.T) {
	var lens []int
	ad := &scriptAdapter{fn: func(n int, messages []string) (CompletionResult, error) {
		lens = append(lens, len(messages))
		if n == 1 {
			return CompletionResult{
				FinishReason: tool.ToolCalls,
				ToolCalls:    []ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}},
			}, nil
		}
		return CompletionResult{Content: "done"}, nil
	}}
	echo := tool.NewTool[struct{}]("echo", "d",
		func(ctx context.Context, _, _ string, _ struct{}) (context.Context, string, error) {
			return ctx, "tool-out", nil
		})
	opts := &RunOptions[string]{
		Tools: []tool.Tool{echo},
		CompactContext: func(_ context.Context, messages []string, reason CompactReason) ([]string, error) {
			if reason == CompactThreshold && len(messages) > 3 {
				return messages[len(messages)-3:], nil
			}
			return messages, nil
		},
	}
	_, _, _, err := RunChatCompletion(context.Background(), ad, []string{"sys", "u"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(lens) != 2 {
		t.Fatalf("Complete 次数 = %d", len(lens))
	}
	if lens[0] != 2 {
		t.Fatalf("首轮 LLM 消息数 = %d，期望 2（sys+user）", lens[0])
	}
	if lens[1] != 3 {
		t.Fatalf("工具后压缩再请求消息数 = %d，期望 3", lens[1])
	}
}

func TestOverflowSameLengthStillRetries(t *testing.T) {
	ad := &scriptAdapter{fn: func(n int, messages []string) (CompletionResult, error) {
		if n == 1 {
			return CompletionResult{}, errors.New("prompt is too long")
		}
		if len(messages) != 2 || messages[0] != "sum" || messages[1] != "u2" {
			t.Fatalf("重试消息 = %v，期望 [sum u2]（条数不变）", messages)
		}
		return CompletionResult{Content: "ok"}, nil
	}}
	_, out, _, err := RunChatCompletion(context.Background(), ad, []string{"u1", "u2"}, &RunOptions[string]{
		CompactContext: func(_ context.Context, messages []string, reason CompactReason) ([]string, error) {
			if reason == CompactOverflow {
				return []string{"sum", "u2"}, nil
			}
			return messages, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ad.n != 2 {
		t.Fatalf("Complete 次数 = %d", ad.n)
	}
	if out[len(out)-1] != "ok" {
		t.Fatalf("out = %v", out)
	}
}
