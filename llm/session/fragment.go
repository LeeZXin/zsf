package session

import "strings"

// 上下文片段标记：注入历史的 user 消息，不进 system prompt，
// 避免改 ExtraPrompt / 摘要拼进 system 打崩前缀缓存。
const (
	// CompactFragmentOpen / Close 压缩摘要片段（替代把摘要拼进 system）。
	CompactFragmentOpen  = "<compaction-summary>"
	CompactFragmentClose = "</compaction-summary>"
	// InterruptedTurnGuidance 用户中断当前轮后写入历史的模型可见说明。
	InterruptedTurnGuidance = "<turn-aborted>\n本次执行被用户中断。不要假设被中断的操作已经完成；根据已得到的工具结果继续，或询问用户下一步。\n</turn-aborted>"
)

// maxCompactFragmentChars 压缩摘要写入历史的上限（有界注入）。
const maxCompactFragmentChars = 32000

// FormatCompactFragment 把摘要包成历史中的 user 片段。
func FormatCompactFragment(summary string) string {
	if summary == "" {
		return ""
	}
	if len(summary) > maxCompactFragmentChars {
		summary = summary[:maxCompactFragmentChars] + "\n[摘要过长已截断]"
	}
	return CompactFragmentOpen + "\n" + summary + "\n" + CompactFragmentClose
}

// IsContextFragment 是否为 harness 注入的上下文片段（回放/裁剪时识别）。
func IsContextFragment(content string) bool {
	return strings.Contains(content, CompactFragmentOpen) ||
		strings.Contains(content, "<turn-aborted>") ||
		strings.Contains(content, "<memory-update>") ||
		strings.Contains(content, "<skills-update>") ||
		strings.Contains(content, "<approved-command-prefix>") ||
		strings.Contains(content, "<workspace-context>") ||
		strings.Contains(content, "<agents-md-update>") ||
		strings.Contains(content, "<collaboration_mode>")
}
