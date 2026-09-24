// Package hitl 人机协同（Human-in-the-loop）原语：WaitForApproval 挂起当前
// 工具调用等待用户反馈，经分类器归为同意/拒绝/修改；steering 通道经 ctx 注入，
// 仅在与 Session 联动的执行路径内可用。
package hitl

import (
	"context"
	"errors"
	"strings"
)

// HumanEvent 人类反馈分类：HITL 等待中用户反馈的语义类型。
type HumanEvent string

const (
	// HumanEventApprove 用户同意执行。
	HumanEventApprove HumanEvent = "approve"
	// HumanEventApproveAlways 用户同意执行并记住同类操作（后续同类调用
	// 免审批）。「记住」的具体语义由调用方实现（如注册会话级 allow 规则）。
	HumanEventApproveAlways HumanEvent = "approve_always"
	// HumanEventReject 用户拒绝执行，反馈文本为拒绝原因/态度。
	HumanEventReject HumanEvent = "reject"
	// HumanEventModify 用户要求修改后执行，反馈文本为修改意见。
	HumanEventModify HumanEvent = "modify"
)

// HumanEventClassifier 将用户反馈分类为事件类型与反馈文本
// （协议噪音如分类前缀已剥离）。
type HumanEventClassifier func(input string) (HumanEvent, string)

// prefixEvents 反馈分类前缀约定（顺序即优先级）。
var prefixEvents = []struct {
	prefix string
	event  HumanEvent
}{
	{"approve:", HumanEventApprove},
	{"always:", HumanEventApproveAlways},
	{"reject:", HumanEventReject},
	{"modify:", HumanEventModify},
}

// DefaultHumanEventClassifier 默认反馈分类器：
//   - 带 "approve:" / "reject:" / "modify:" 前缀（忽略大小写与首尾空白）
//     时按前缀分类，冒号后内容为反馈文本（业务方把用户按钮点击编码为
//     前缀文本经 Session.Send 送入，即获得结构化反馈语义）；
//   - 无前缀时：输入包含"同意"/"确认"或为 y/yes/ok 判定为同意，
//     其余判定为拒绝（拒绝保留原文，供模型感知用户态度）。
func DefaultHumanEventClassifier(input string) (HumanEvent, string) {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	for _, p := range prefixEvents {
		if rest, ok := strings.CutPrefix(lower, p.prefix); ok {
			return p.event, strings.TrimSpace(rest)
		}
	}
	if strings.Contains(lower, "同意") ||
		strings.Contains(lower, "确认") ||
		lower == "y" || lower == "yes" || lower == "ok" {
		return HumanEventApprove, trimmed
	}
	return HumanEventReject, trimmed
}

// ErrSteerChanUnavailable steering 通道不可用：ctx 中无 WithSteerChan 注入的通道
// （工具未运行在 Session 执行路径内，或通道已被关闭）。
// 调用方通常应把该错误作为工具结果返回给模型，而不是中断会话。
var ErrSteerChanUnavailable = errors.New("llm: steer channel unavailable")

// WaitForApproval 工具内的 HITL 原语：挂起当前工具调用，等待用户在会话中的反馈。
// 典型用法：shell 等高风险工具在真正执行前调用本函数，
// 用户反馈后（同意/拒绝/修改）再继续往下走。
//
// 行为：
//   - 从 steering 通道读取一条用户消息，经分类器分类后返回事件类型与反馈文本；
//   - 等待期间会话保持执行态，用户经 Session.Send 发送的消息直达本函数，
//     不会被注入下一轮对话；
//   - ctx 取消（Session.Cancel/Close 或工具自身包的超时 ctx）返回 ctx.Err()；
//   - ctx 中无 steering 通道（引擎独立使用该工具）或通道已关闭时
//     返回 ErrSteerChanUnavailable。
//
// 本函数只负责等待与分类，不发布任何事件——告知用户"正在等什么审批"
// 由调用方负责（如 approval 装饰器发出 EventApprovalRequested，带可读描述）。
// classifier 缺省或为 nil 时使用 DefaultHumanEventClassifier。
func WaitForApproval(ctx context.Context, classifier ...HumanEventClassifier) (HumanEvent, string, error) {
	ch, ok := SteerChanFromContext(ctx)
	if !ok {
		return "", "", ErrSteerChanUnavailable
	}
	var input string
	select {
	case v, ok := <-ch:
		if !ok {
			return "", "", ErrSteerChanUnavailable
		}
		input = v
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	c := DefaultHumanEventClassifier
	if len(classifier) > 0 && classifier[0] != nil {
		c = classifier[0]
	}
	event, text := c(input)
	return event, text, nil
}

// WaitForInput 从 steering 通道读取一条原始用户消息（不做同意/拒绝分类）。
// 供 request_user_input 等结构化提问：整段回复原样交给模型。
// ctx 取消或通道不可用时的错误语义与 WaitForApproval 相同。
func WaitForInput(ctx context.Context) (string, error) {
	ch, ok := SteerChanFromContext(ctx)
	if !ok {
		return "", ErrSteerChanUnavailable
	}
	select {
	case v, ok := <-ch:
		if !ok {
			return "", ErrSteerChanUnavailable
		}
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type steerChanContextKey struct{}

// WithSteerChan 将 steering 通道注入 ctx（Session 在每轮执行时自动注入），
// 供工具在"当前工具调用内"等待用户确认等 HITL 场景使用。
func WithSteerChan(ctx context.Context, ch <-chan string) context.Context {
	return context.WithValue(ctx, steerChanContextKey{}, ch)
}

// SteerChanFromContext 从 ctx 取 steering 通道（Session 执行路径内可用）。
func SteerChanFromContext(ctx context.Context) (<-chan string, bool) {
	ch, ok := ctx.Value(steerChanContextKey{}).(<-chan string)
	return ch, ok && ch != nil
}
