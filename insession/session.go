// Package insession 提供用户会话管理相关的工具函数。
// 负责从 HTTP 请求头中提取当前登录用户信息和操作者信息，
// 并提供超级管理员权限校验的 Gin 中间件。
package insession

import (
	"net/http"
	"strings"

	"github.com/LeeZXin/zsf/rpc"

	"github.com/gin-gonic/gin"
)

// Operator 表示当前操作者的信息。
type Operator struct {
	Account  string `json:"account"`  // 操作者账号
	ClientIp string `json:"clientIp"` // 操作者客户端 IP 地址
}

// GetOperator 从 Gin 上下文中提取当前操作者的完整信息。
// 账号信息从请求头中获取，客户端 IP 从特定的 RPC 请求头中获取。
// 参数：
//   - c: Gin 上下文
//
// 返回：
//   - Operator 结构体，包含账号和客户端 IP
func GetOperator(c *gin.Context) Operator {
	return Operator{
		Account:  GetAccount(c),
		ClientIp: c.GetHeader(rpc.ClientIp),
	}
}

// GetAccount 从 Gin 上下文的请求头中获取当前登录用户的账号。
// 参数：
//   - c: Gin 上下文
//
// 返回：
//   - 用户账号字符串，如果未登录则为空字符串
func GetAccount(c *gin.Context) string {
	return c.GetHeader(rpc.Account)
}

func IsSuper(c *gin.Context) bool {
	return c.GetHeader(rpc.Super) == "1"
}

// IsSuperFilter 是 Gin 的认证中间件，用于校验当前用户是否为超级管理员。
// 仅校验网关注入的请求头 X-Super 是否为 "1"（不做任何数据库/远程查询），
// 校验不通过则终止请求并返回 403；应挂在需要超管权限的路由上。
// 注意：本中间件假定网关/鉴权层已保证该请求头不可伪造。
func IsSuperFilter(c *gin.Context) {
	if !IsSuper(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

// AuthFilter 是 Gin 的认证中间件。
// 它会检查当前请求的路径是否在免认证列表中，如果在则直接放行；
// 否则检查当前会话中是否已有登录账户信息，如果没有则返回 401 未授权状态码。
func AuthFilter(skipPaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(skipPaths) > 0 {
			path := c.Request.URL.Path
			for _, skipPath := range skipPaths {
				if strings.HasPrefix(path, skipPath) {
					c.Next()
					return
				}
			}
		}
		if GetAccount(c) == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
