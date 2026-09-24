package session

import (
	"context"
	"slices"

	"github.com/LeeZXin/zsf/llm/engine"
	"github.com/LeeZXin/zsf/llm/tool"
)

const (
	roleSystem    = "system"
	roleAssistant = "assistant"
	roleTool      = "tool"
)

// compactContext 引擎 CompactContext 钩子：阈值压缩在每次 LLM 调用前执行
// （含同一轮工具循环内部）；overflow 时即使未超配置上限也强制腾窗口。
// 未丢弃任何消息时原样返回入参（不改写 system，以免把 ExtraPrompt 提前到本轮）。
func (s *Session[M]) compactContext(ctx context.Context, messages []M, reason engine.CompactReason) ([]M, error) {
	_, body := splitSystem(s.adapter, messages)
	before := len(body)
	oldSummary := s.summary
	var err error
	body, err = s.compactBody(ctx, body, reason == engine.CompactOverflow)
	if err != nil {
		return nil, err
	}
	if len(body) == before && s.summary == oldSummary {
		if reason == engine.CompactOverflow {
			return nil, engine.ErrCannotCompact
		}
		return messages, nil
	}
	s.contextTokens.Store(s.estimateTokens(body))
	return joinSystem(s.adapter, s.effectivePrompt(), body), nil
}

// compactBody 压缩不含 system 的消息体。force 时若阈值裁剪未丢弃，再拆一轮/当前轮前缀。
func (s *Session[M]) compactBody(ctx context.Context, body []M, force bool) ([]M, error) {
	n := len(body)
	body, err := s.trim(ctx, body)
	if err != nil {
		return nil, err
	}
	if !force || len(body) < n {
		return body, nil
	}
	dropped, kept := s.splitForForce(body)
	if len(dropped) == 0 {
		return body, engine.ErrCannotCompact
	}
	return s.drop(ctx, dropped, kept)
}

// splitForForce overflow 强制腾窗口：优先丢掉最早一轮 user 对话；
// 只剩当前一轮时丢掉该轮中最后一次 assistant 之前的工具轨迹（保留 user + 最近 assistant/tool）。
func (s *Session[M]) splitForForce(body []M) (dropped, kept []M) {
	users := s.roleIndexes(body, tool.User)
	if len(users) >= 2 {
		cut := s.alignKeptStart(body, users[1])
		if cut > 0 && cut < len(body) {
			return body[:cut], body[cut:]
		}
	}
	lastUser := s.lastRoleIndex(body, tool.User)
	lastAsst := s.lastRoleIndex(body, roleAssistant)
	if lastUser >= 0 && lastAsst > lastUser+1 {
		keepFrom := s.alignKeptStart(body, lastAsst)
		if keepFrom > lastUser+1 && keepFrom < len(body) {
			dropped = body[lastUser+1 : keepFrom]
			kept = append(slices.Clone(body[:lastUser+1]), body[keepFrom:]...)
			return dropped, kept
		}
	}
	return nil, body
}

// clipToTokenLimit 按 token 上限裁剪：优先丢掉更早的 user 轮；
// 当前一轮本身超限时保留最后一条 user，丢掉该轮中较早的 assistant/tool（不拆 tool pair）。
func (s *Session[M]) clipToTokenLimit(messages []M, limit uint64) (dropped, kept []M) {
	if limit == 0 || s.estimateTokens(messages) <= limit {
		return nil, messages
	}
	lastUser := s.lastRoleIndex(messages, tool.User)
	cut := 0
	for cut < len(messages)-1 && s.estimateTokensFrom(cut, messages) > limit {
		cut++
	}
	cut = s.alignKeptStart(messages, cut)
	if lastUser >= 0 && cut > lastUser {
		keepFrom := s.lastRoleIndex(messages, roleAssistant)
		if keepFrom <= lastUser+1 {
			return nil, messages
		}
		keepFrom = s.alignKeptStart(messages, keepFrom)
		if keepFrom <= lastUser+1 || keepFrom >= len(messages) {
			return nil, messages
		}
		dropped = messages[lastUser+1 : keepFrom]
		kept = append(slices.Clone(messages[:lastUser+1]), messages[keepFrom:]...)
		return dropped, kept
	}
	if cut <= 0 || cut >= len(messages) {
		return nil, messages
	}
	return messages[:cut], messages[cut:]
}

// alignKeptStart 把裁剪起点对齐到非 tool 消息：不得留下没有对应 assistant tool_calls 的孤儿 tool 结果。
func (s *Session[M]) alignKeptStart(messages []M, cut int) int {
	for cut < len(messages) && s.adapter.MessageRole(messages[cut]) == roleTool {
		cut++
	}
	return cut
}

func (s *Session[M]) lastRoleIndex(messages []M, role string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if s.adapter.MessageRole(messages[i]) == role {
			return i
		}
	}
	return -1
}

func (s *Session[M]) roleIndexes(messages []M, role string) []int {
	var idx []int
	for i, m := range messages {
		if s.adapter.MessageRole(m) == role {
			idx = append(idx, i)
		}
	}
	return idx
}

func splitSystem[M any](adapter engine.Adapter[M], messages []M) (sys, body []M) {
	if len(messages) > 0 && adapter.MessageRole(messages[0]) == roleSystem {
		return messages[:1], messages[1:]
	}
	return nil, messages
}

func joinSystem[M any](adapter engine.Adapter[M], prompt string, body []M) []M {
	if prompt == "" {
		return body
	}
	return append([]M{adapter.ConvertToSystemMessage(prompt)}, body...)
}

// messagesDelta 本轮相对压缩前历史的增量：orig 的最长后缀若为 current 的前缀则切掉该前缀；
// 轮内压缩丢掉 orig 前缀后仍能定位本轮新增（含 user/assistant/tool）。
func (s *Session[M]) messagesDelta(orig, current []M) []M {
	for k := 0; k <= len(orig); k++ {
		prefix := orig[k:]
		if len(current) < len(prefix) {
			continue
		}
		if s.messagesEqual(prefix, current[:len(prefix)]) {
			return current[len(prefix):]
		}
	}
	return current
}

func (s *Session[M]) messagesEqual(a, b []M) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if s.adapter.MessageRole(a[i]) != s.adapter.MessageRole(b[i]) {
			return false
		}
		if s.adapter.MessageContent(a[i]) != s.adapter.MessageContent(b[i]) {
			return false
		}
	}
	return true
}
