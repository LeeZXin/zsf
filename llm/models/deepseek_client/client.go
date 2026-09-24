// Package deepseek_client DeepSeek 客户端适配：配置加载、请求构造、
// engine.Adapter 实现（含流式）、无状态调用与 Session 便捷构造。
package deepseek_client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/llm/engine"
	"github.com/LeeZXin/zsf/llm/tool"

	"github.com/LeeZXin/zsf/utils/listutil"
	"github.com/cohesion-org/deepseek-go"
	"github.com/cohesion-org/deepseek-go/constants"
)

// newChatCompletionRequest 构造非流式对话补全请求，model 为空时使用 DeepSeekV4Pro。
func newChatCompletionRequest(messages []deepseek.ChatCompletionMessage, opts *engine.Options, model string, tools []tool.Tool) *deepseek.ChatCompletionRequest {
	if model == "" {
		model = deepseek.DeepSeekV4Pro
	}
	return &deepseek.ChatCompletionRequest{
		EnableThinking: opts.EnableThinking,
		Model:          model,
		Messages:       messages,
		Tools: listutil.MapNe(tools, func(t tool.Tool) deepseek.Tool {
			return t.GetDeepseekTool()
		}),
		ToolChoice:       "auto",
		TopP:             opts.TopP,
		Temperature:      opts.Temperature,
		FrequencyPenalty: opts.FrequencyPenalty,
		MaxTokens:        opts.MaxTokens,
		PresencePenalty:  opts.PresencePenalty,
		Stop:             opts.Stop,
		LogProbs:         opts.LogProbs,
		TopLogProbs:      opts.TopLogProbs,
	}
}

// newStreamChatCompletionRequest 构造流式对话补全请求，开启 usage 回传。
func newStreamChatCompletionRequest(messages []deepseek.ChatCompletionMessage, opts *engine.Options, model string, tools []tool.Tool) *deepseek.StreamChatCompletionRequest {
	if model == "" {
		model = deepseek.DeepSeekV4Pro
	}
	return &deepseek.StreamChatCompletionRequest{
		EnableThinking: opts.EnableThinking,
		Model:          model,
		Messages:       messages,
		StreamOptions:  deepseek.StreamOptions{IncludeUsage: true},
		Tools: listutil.MapNe(tools, func(t tool.Tool) deepseek.Tool {
			return t.GetDeepseekTool()
		}),
		TopP:             opts.TopP,
		Temperature:      opts.Temperature,
		FrequencyPenalty: opts.FrequencyPenalty,
		MaxTokens:        opts.MaxTokens,
		PresencePenalty:  opts.PresencePenalty,
		Stop:             opts.Stop,
		LogProbs:         opts.LogProbs,
		TopLogProbs:      opts.TopLogProbs,
	}
}

// adapter 将 deepseek SDK 适配到 engine.Adapter，
// 工具循环、回调、轮次上限等编排逻辑由引擎统一处理。
type adapter struct {
	client *deepseek.Client
	opts   *engine.Options
}

func NewAdapter(apiKey string, opts *engine.Options) engine.Adapter[deepseek.ChatCompletionMessage] {
	if opts == nil {
		opts = new(engine.Options)
	}
	if opts.DefaultModel == "" {
		opts.DefaultModel = deepseek.DeepSeekV4Pro
	}
	if opts.CompressModel == "" {
		opts.CompressModel = deepseek.DeepSeekV4Flash
	}
	if opts.RetryMax == 0 {
		opts.RetryMax = defaultRetryMax
	}
	var client *deepseek.Client
	if opts.BaseURL != "" {
		client = deepseek.NewClient(apiKey, opts.BaseURL)
	} else {
		client = deepseek.NewClient(apiKey)
	}
	return &adapter{client: client, opts: opts}
}

func (a *adapter) Complete(ctx context.Context, messages []deepseek.ChatCompletionMessage, model string, tools []tool.Tool) (engine.CompletionResult, error) {
	req := newChatCompletionRequest(messages, a.opts, model, tools)
	var resp *deepseek.ChatCompletionResponse
	var err error
	for attempt := 0; ; attempt++ {
		resp, err = a.client.CreateChatCompletion(ctx, req)
		delay, ok := retryDelay(err, a.opts, attempt)
		if !ok {
			break
		}
		if err := sleepCtx(ctx, delay); err != nil {
			return engine.CompletionResult{}, err
		}
	}
	if err != nil {
		return engine.CompletionResult{}, err
	}
	ret := engine.CompletionResult{
		Usage: engine.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		HasUsage: true,
		Raw:      resp,
	}
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		ret.Content = choice.Message.Content
		ret.ReasoningContent = choice.Message.ReasoningContent
		ret.FinishReason = choice.FinishReason
		for _, tc := range choice.Message.ToolCalls {
			ret.ToolCalls = append(ret.ToolCalls, engine.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}
	return ret, nil
}

func (a *adapter) OpenStream(ctx context.Context, messages []deepseek.ChatCompletionMessage, model string, tools []tool.Tool) (engine.StreamReader, error) {
	req := newStreamChatCompletionRequest(messages, a.opts, model, tools)
	var stream deepseek.ChatCompletionStream
	var err error
	for attempt := 0; ; attempt++ {
		stream, err = a.client.CreateChatCompletionStream(ctx, req)
		delay, ok := retryDelay(err, a.opts, attempt)
		if !ok {
			break
		}
		if err := sleepCtx(ctx, delay); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create chat stream completion: %w", err)
	}
	return &engine.StreamReaderFn{
		RecvFn: func() (*engine.StreamChunk, error) {
			chunk, err := stream.Recv()
			if err != nil {
				return nil, err
			}
			ret := &engine.StreamChunk{Raw: chunk}
			for _, choice := range chunk.Choices {
				if choice.Index != 0 {
					continue
				}
				ret.Content = choice.Delta.Content
				ret.ReasoningContent = choice.Delta.ReasoningContent
				if choice.FinishReason != "" {
					ret.FinishReason = choice.FinishReason
				}
				for _, toolCall := range choice.Delta.ToolCalls {
					ret.ToolCalls = append(ret.ToolCalls, engine.ToolCallDelta{
						Index:     toolCall.Index,
						ID:        toolCall.ID,
						Type:      toolCall.Type,
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					})
				}
			}
			if chunk.Usage != nil {
				ret.Usage = &engine.Usage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
			}
			return ret, nil
		},
		CloseFn: stream.Close,
	}, nil
}

// defaultRetryMax 429/5xx 默认重试次数（Options.RetryMax 为 0 时启用）。
const defaultRetryMax = 2

// retryDelay 判定错误是否可重试：SDK APIError 的 429/5xx 返回指数退避
// 时长（base*2^attempt，base 默认 1s），其余错误返回不重试。
// attempt 从 0 起；RetryMax 负数禁用。
func retryDelay(err error, opts *engine.Options, attempt int) (time.Duration, bool) {
	if opts == nil || attempt >= opts.RetryMax || err == nil {
		return 0, false
	}
	var apiErr *deepseek.APIError
	if !errors.As(err, &apiErr) {
		return 0, false
	}
	switch apiErr.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		base := opts.RetryBaseDelay
		if base <= 0 {
			base = time.Second
		}
		return base << attempt, true
	}
	return 0, false
}

// sleepCtx 可取消的退避等待：ctx 取消优先于等待结束。
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *adapter) ConvertToAssistantMessage(res engine.CompletionResult) deepseek.ChatCompletionMessage {
	return deepseek.ChatCompletionMessage{
		Role:             constants.ChatMessageRoleAssistant,
		Content:          res.Content,
		ReasoningContent: res.ReasoningContent,
		ToolCalls: listutil.MapNe(res.ToolCalls, func(toolCall engine.ToolCall) deepseek.ToolCall {
			return deepseek.ToolCall{
				Type: "function",
				ID:   toolCall.ID,
				Function: deepseek.ToolCallFunction{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			}
		}),
	}
}

func (a *adapter) MessageRole(message deepseek.ChatCompletionMessage) string {
	return message.Role
}

func (a *adapter) MessageContent(message deepseek.ChatCompletionMessage) string {
	if len(message.ToolCalls) == 0 {
		return message.Content
	}
	var b strings.Builder
	b.WriteString(message.Content)
	for _, tc := range message.ToolCalls {
		b.WriteByte(' ')
		b.WriteString(tc.Function.Name)
		b.WriteString(tc.Function.Arguments)
	}
	return b.String()
}

func (a *adapter) ConvertToUserMessage(content string) deepseek.ChatCompletionMessage {
	return deepseek.ChatCompletionMessage{
		Role:    constants.ChatMessageRoleUser,
		Content: content,
	}
}

func (a *adapter) ConvertToSystemMessage(content string) deepseek.ChatCompletionMessage {
	return deepseek.ChatCompletionMessage{
		Role:    constants.ChatMessageRoleSystem,
		Content: content,
	}
}

func (a *adapter) ConvertToToolMessage(toolCall engine.ToolCall, content string) deepseek.ChatCompletionMessage {
	return deepseek.ChatCompletionMessage{
		Role:       constants.ChatMessageRoleTool,
		ToolCallID: toolCall.ID,
		Content:    content,
	}
}

// ClearReasoningContent 清空消息中的思考链，返回清理后的切片。
// 不修改入参及其共享底层数据：全部消息无需清空时原样返回（零拷贝），
// 否则克隆整个切片后清空——已交给 OnMessageAppend 的完整消息不受影响，
// 回调方可安全异步消费。
func (a *adapter) ClearReasoningContent(messages []deepseek.ChatCompletionMessage) []deepseek.ChatCompletionMessage {
	needClear := false
	for i := range messages {
		if messages[i].ReasoningContent != "" {
			needClear = true
			break
		}
	}
	if !needClear {
		return messages
	}
	out := slices.Clone(messages)
	for i := range out {
		out[i].ReasoningContent = ""
	}
	return out
}

func (a *adapter) DefaultModel() string {
	return a.opts.DefaultModel
}

func (a *adapter) CompressModel() string {
	return a.opts.CompressModel
}

func (a *adapter) DefaultTools() []tool.Tool {
	return a.opts.DefaultTools
}
