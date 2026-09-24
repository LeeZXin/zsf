package engine

import (
	"time"

	"github.com/LeeZXin/zsf/llm/tool"
)

// Options 对话补全请求参数，零值表示使用默认值（除 Tools 外）。
type Options struct {
	TopP             float32
	Temperature      float32
	FrequencyPenalty float32
	MaxTokens        int
	PresencePenalty  float32
	Stop             []string
	LogProbs         bool
	TopLogProbs      int
	EnableThinking   bool
	DefaultModel     string
	CompressModel    string
	DefaultTools     []tool.Tool
	// RetryMax 429/5xx 错误的最大重试次数（请求打开阶段，指数退避）。
	// 0 由 NewAdapter 兜底为默认 2；负数禁用重试。
	RetryMax int
	// RetryBaseDelay 重试退避基础间隔（第 n 次重试等待 base*2^n）；
	// <=0 使用默认 1s。
	RetryBaseDelay time.Duration
	// 连接域名
	BaseURL string
}
