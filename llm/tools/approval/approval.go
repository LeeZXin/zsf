// Package approval 提供需要用户确认才能执行的工具装饰器：
// 高权限工具（shell、删除、变更类）经 WithApproval 包装后，
// 执行前阻塞等待用户在会话中反馈——确认行为发生在当前对话的
// 当前工具调用内（经 steering 通道），不需要模型为确认多思考一轮。
//
// 审批告知不嵌入对话消息流：装饰器经独立的事件通道发出
// EventApprovalRequested（带工具名、参数与可选的可读描述），
// 业务方订阅后向前端展示"有什么动作需要审批"；用户决定经
// Session.Send 送入 steering 通道，直达本工具。
package approval

import (
	"context"
	"time"

	"github.com/LeeZXin/zsf/llm/event"
	"github.com/LeeZXin/zsf/llm/hitl"
	"github.com/LeeZXin/zsf/llm/tool"

	"github.com/cohesion-org/deepseek-go"
)

// WithApproval 包装工具：Invoke 前经独立事件通道发出审批请求
// （EventApprovalRequested），再经 hitl.WaitForApproval 阻塞等待用户反馈。
//
// 反馈处理（按 HumanEvent 分类）：
//   - approve：委托原工具执行；
//   - reject：拒绝文案作为工具结果返回（不报错），模型感知用户态度并调整；
//   - modify：修改意见作为工具结果返回，由模型按意见重新调用工具——
//     装饰器不替模型改写参数；
//
// 参数：
//   - classifier 反馈分类器，nil 时使用 hitl.DefaultHumanEventClassifier；
//   - describe 将原始参数转为向用户展示的可读审批描述（如命令原文），
//     nil 时事件只带工具名与原始参数。
//
// 约束与约定：
//   - 仅在与 Session 联动的执行路径内可用（等待依赖 steering 通道注入）；
//     引擎独立使用该工具时返回 hitl.ErrSteerChanUnavailable；
//   - 等待期间会话保持执行态，用户经 Session.Send 发送的消息直达本工具，
//     不会被注入下一轮对话；
//   - 等待超时由原工具自身控制（Invoke 内对 ctx 包一层 context.WithTimeout）。
func WithApproval(tool tool.Tool, classifier hitl.HumanEventClassifier, describe func(arguments string) string) tool.Tool {
	if classifier == nil {
		classifier = hitl.DefaultHumanEventClassifier
	}
	return &approvalTool{inner: tool, classifier: classifier, describe: describe}
}

type approvalTool struct {
	inner      tool.Tool
	classifier hitl.HumanEventClassifier
	describe   func(arguments string) string
}

func (t *approvalTool) Invoke(ctx context.Context, toolCallID, toolCallName, arguments string) (context.Context, string, error) {
	// 审批告知走独立事件通道，不嵌入对话消息流：
	// 前端从事件流得知"有什么动作需要审批"，展示并收集用户决定。
	if sink, ok := event.SinkFromContext(ctx); ok {
		ev := event.Event{
			Type:       event.ApprovalRequested,
			Timestamp:  time.Now(),
			ToolCallID: toolCallID,
			ToolName:   toolCallName,
			Arguments:  arguments,
		}
		if t.describe != nil {
			ev.Content = t.describe(arguments)
		}
		sink.Emit(ev)
	}
	event, text, err := hitl.WaitForApproval(ctx, t.classifier)
	if err != nil {
		return ctx, "", err
	}
	switch event {
	case hitl.HumanEventApprove, hitl.HumanEventApproveAlways:
		// always 的「记住同类操作」由调用方实现（如会话级 allow 规则）；
		// 本装饰器只保证本次调用会真正执行，不再误当成拒绝。
		return t.inner.Invoke(ctx, toolCallID, toolCallName, arguments)
	case hitl.HumanEventModify:
		// 修改意见返回给模型，让模型按意见重新调用工具。
		return ctx, "用户要求修改：" + text, nil
	default:
		// 拒绝信息作为工具结果返回，模型能看到用户的态度并调整。
		return ctx, "用户拒绝执行：" + text, nil
	}
}

func (t *approvalTool) GetDeepseekTool() deepseek.Tool { return t.inner.GetDeepseekTool() }
func (t *approvalTool) GetFunctionName() string        { return t.inner.GetFunctionName() }
