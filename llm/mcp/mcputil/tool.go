package mcputil

import (
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool 封装 mcp 工具定义与注册方式。
type Tool struct {
	*mcp.Tool
	// Handler 低层用法：自定义 InputSchema 与原始 mcp.ToolHandler。
	// 通过 NewTool 或 NewTypedTool 创建时该字段为 nil。
	Handler mcp.ToolHandler
	// addFunc 类型化注册入口，由 NewTypedTool 填充，
	// 借助 mcp.AddTool 自动推断输入/输出 schema 并处理参数校验与结果打包。
	addFunc func(*mcp.Server)
}

// Add 将工具注册到 mcp server。
func (t Tool) Add(s *mcp.Server) {
	if t.addFunc != nil {
		t.addFunc(s)
		return
	}
	s.AddTool(t.Tool, t.Handler)
}

// NewTool 创建带自定义 InputSchema 的 mcp 工具。
// 参数仅用于推断 schema，handler 自行解析与校验参数。
func NewTool[In any](name, description string, handler mcp.ToolHandler) Tool {
	if name == "" {
		panic("mcp.tool: name not set")
	}
	if description == "" {
		panic("mcp.tool: description not set")
	}
	if handler == nil {
		panic("mcp.tool: handler not set")
	}
	var in In
	v := reflect.ValueOf(in)
	if v.Kind() != reflect.Struct {
		panic("mcp.tool: expected struct")
	}
	inputSchema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	return Tool{
		Tool: &mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: inputSchema,
		},
		Handler: handler,
	}
}

// NewTypedTool 创建类型化 mcp 工具：
//   - In 自动推断输入 schema，参数自动反序列化并校验；
//   - Out 自动作为结构化输出返回，无需手动打包 CallToolResult。
func NewTypedTool[In, Out any](name, description string, handler mcp.ToolHandlerFor[In, Out]) Tool {
	if name == "" {
		panic("mcp.tool: name not set")
	}
	if description == "" {
		panic("mcp.tool: description not set")
	}
	if handler == nil {
		panic("mcp.tool: handler not set")
	}
	t := &mcp.Tool{
		Name:        name,
		Description: description,
	}
	return Tool{
		Tool:    t,
		addFunc: func(s *mcp.Server) { mcp.AddTool(s, t, handler) },
	}
}
