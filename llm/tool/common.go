// Package tool LLM 工具抽象：一次注册（NewTool）自动推导三家 SDK 的
// function schema 与参数反序列化，业务能力以工具形态接入引擎。
package tool

import (
	"context"
	"reflect"

	"github.com/bytedance/sonic"
	"github.com/cohesion-org/deepseek-go"
	"github.com/google/jsonschema-go/jsonschema"
)

const (
	ToolCalls = "tool_calls"
	User      = "user"
)

// Invoker 工具调用器：接收反序列化后的入参 T，
// 返回（可能被增强的）context、结果字符串与错误。
// 前两个 string 参数依次为工具调用 ID 与工具名。
type Invoker[T any] func(context.Context, string, string, T) (context.Context, string, error)

// Tool LLM 工具的统一抽象：一次注册（NewTool），
// 即可同时用于 deepseek / qwen / ark 三家的工具调用。
type Tool interface {
	// Invoke 执行工具，arguments 为 JSON 字符串，内部反序列化后交给 Invoker。
	Invoke(context.Context, string, string, string) (context.Context, string, error)
	// GetDeepseekTool 返回 deepseek SDK 格式的工具定义。
	GetDeepseekTool() deepseek.Tool
	// GetFunctionName 返回工具名，用于匹配模型返回的 tool call。
	GetFunctionName() string
}

// ParallelAware 可选接口：工具声明是否可与其它工具并行执行。
// 未实现时引擎默认并行（现有工具行为不变）。
// 写文件、交互式 HITL、共享可变状态的工具应返回 false：引擎对其取
// 互斥锁，只读工具仍可并行。
type ParallelAware interface {
	SupportsParallel() bool
}

// SupportsParallel 报告工具是否允许并行。未实现 ParallelAware 时为 true。
func SupportsParallel(t Tool) bool {
	if p, ok := t.(ParallelAware); ok {
		return p.SupportsParallel()
	}
	return true
}

// exclusiveTool 把内嵌工具标为互斥（SupportsParallel=false）。
type exclusiveTool struct{ Tool }

// SupportsParallel 实现 ParallelAware。
func (exclusiveTool) SupportsParallel() bool { return false }

// Exclusive 包装工具为互斥执行：与其它互斥工具、以及正在运行的并行
// 工具都不会重叠（引擎 RW 锁：互斥取写锁，并行取读锁）。
func Exclusive(t Tool) Tool {
	if t == nil {
		return nil
	}
	if !SupportsParallel(t) {
		return t
	}
	return exclusiveTool{Tool: t}
}

// toolImpl Tool 的泛型实现，NewTool 构造时一次性生成三家 SDK 的工具定义。
type toolImpl[T any] struct {
	Name         string
	DeepseekTool deepseek.Tool
	Invoker      Invoker[T]
}

func (tool *toolImpl[T]) Invoke(ctx context.Context, toolCallID, toolCallName, arguments string) (context.Context, string, error) {
	var t T
	err := sonic.Unmarshal([]byte(arguments), &t)
	if err != nil {
		return ctx, "", err
	}
	return tool.Invoker(ctx, toolCallID, toolCallName, t)
}

func (tool *toolImpl[T]) GetDeepseekTool() deepseek.Tool {
	return tool.DeepseekTool
}

func (tool *toolImpl[T]) GetFunctionName() string {
	return tool.Name
}

// FunctionSchema 函数参数 JSON Schema（精简版），供各 SDK 的 function 定义使用。
type FunctionSchema struct {
	Type       string                     `json:"type,omitempty"`
	Types      []string                   `json:"types,omitempty"`
	Items      *FunctionSchema            `json:"items,omitempty"`
	Properties map[string]*FunctionSchema `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
}

// convertJSONSchemaToFunctionSchema 将 jsonschema-go 的 Schema 递归转换为 FunctionSchema。
func convertJSONSchemaToFunctionSchema(schema *jsonschema.Schema) *FunctionSchema {
	ret := &FunctionSchema{
		Type:       schema.Type,
		Types:      schema.Types,
		Properties: make(map[string]*FunctionSchema),
		Required:   schema.Required,
	}
	if schema.Items != nil {
		ret.Items = convertJSONSchemaToFunctionSchema(schema.Items)
	}
	if schema.Properties != nil {
		for k, v := range schema.Properties {
			ret.Properties[k] = convertJSONSchemaToFunctionSchema(v)
		}
	}
	return ret
}

// convertJSONSchemaToFunctionParameters 将 jsonschema-go 的 Schema 转换为
// deepseek SDK 的 FunctionParameters。
func convertJSONSchemaToFunctionParameters(schema *jsonschema.Schema) *deepseek.FunctionParameters {
	if schema == nil {
		return nil
	}
	properties := make(map[string]any)
	for k, v := range schema.Properties {
		properties[k] = convertJSONSchemaToFunctionSchema(v)
	}
	return &deepseek.FunctionParameters{
		Type:       schema.Type,
		Properties: properties,
		Required:   schema.Required,
	}
}

// newDeepseekTool 由入参结构体 In 推导 JSON Schema，构建 deepseek 工具定义。
func newDeepseekTool[In any](name, description string) deepseek.Tool {
	if name == "" {
		panic("deepseek.tool: name not set")
	}
	if description == "" {
		panic("deepseek.tool: description not set")
	}
	var in In
	inputType := reflect.TypeOf(in)
	if inputType.Kind() != reflect.Struct {
		panic("deepseek.tool: expected struct")
	}
	inputSchema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	return deepseek.Tool{
		Type: "function",
		Function: deepseek.Function{
			Name:        name,
			Description: description,
			Parameters:  convertJSONSchemaToFunctionParameters(inputSchema),
		},
	}
}

// NewTool 注册一个 LLM 工具：由入参结构体 T 自动推导参数 schema，
// 模型返回的 tool call 参数会反序列化为 T 后交给 invoker 执行。
// 各参数非法时 panic（工具应在包初始化期注册）。
func NewTool[T any](name, description string, invoker Invoker[T]) Tool {
	if name == "" {
		panic("empty name")
	}
	if description == "" {
		panic("empty description")
	}
	if invoker == nil {
		panic("empty invoker")
	}
	return &toolImpl[T]{
		Name:         name,
		DeepseekTool: newDeepseekTool[T](name, description),
		Invoker:      invoker,
	}
}
