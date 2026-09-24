package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/start"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	// mcpClient 包级 MCP 客户端，init 时构建，各会话由它 Connect 派生。
	mcpClient *mcp.Client
)

func init() {
	start.AddInit(func() {
		version := static.GetString("mcp.client.version")
		if version == "" {
			version = "0.0.1"
		}
		mcpClient = mcp.NewClient(&mcp.Implementation{Name: instance.ApplicationName, Version: version}, nil)
	}, -5)
}

// checkEndpoint 校验 endpoint 是否为合法的 http/https 地址。
func checkEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "http://") ||
		strings.HasPrefix(endpoint, "https://")
}

// CreateStreamableSession 以 Streamable HTTP 传输建立 MCP 会话。
// 通过 SetCustomHTTPHeader 挂到 ctx 上的自定义 header 会注入所有请求。
func CreateStreamableSession(ctx context.Context, httpClient *http.Client, endpoint string) (*mcp.ClientSession, error) {
	if !checkEndpoint(endpoint) {
		return nil, fmt.Errorf("invalid endpoint: %s", endpoint)
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}
	return mcpClient.Connect(ctx, transport, nil)
}

// GetCallToolResultFirstTextContent 提取 CallTool 结果中第一段文本内容；
// 结果为空或首段非文本时返回空字符串。
func GetCallToolResultFirstTextContent(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return content.Text
}
