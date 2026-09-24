// Package ginutil 提供基于 Gin 框架的 HTTP 工具函数。
// 包含请求参数绑定、响应封装、文件上传、分页请求/响应结构体等常用功能。
package ginutil

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var (
	// ErrNoForm 表示请求中没有表单字段的错误
	ErrNoForm = errors.New("http: no form field")
)

// BindQuery 将 URL 查询参数绑定到指定的结构体。
// c: Gin 上下文; ptr: 目标结构体指针（需包含 json 标签）
// 返回: 绑定或校验失败的错误
func BindQuery(c *gin.Context, ptr any) error {
	err := binding.MapFormWithTag(ptr, c.Request.URL.Query(), "json")
	if err != nil {
		return err
	}
	return validate(ptr)
}

// BindMultipartForm 将 multipart/form-data 或 x-www-form-urlencoded 表单绑定到结构体。
// c: Gin 上下文; ptr: 目标结构体指针（需包含 json 标签）
// 返回: 绑定或校验失败的错误
func BindMultipartForm(c *gin.Context, ptr any) error {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return err
		}
		if c.Request.MultipartForm == nil {
			return ErrNoForm
		}
		err := binding.MapFormWithTag(ptr, c.Request.MultipartForm.Value, "json")
		if err != nil {
			return err
		}
		return validate(ptr)
	}
	return ErrNoForm
}

// validate 对对象进行校验，如果 Gin 的全局校验器已启用则执行结构体验证。
func validate(obj any) error {
	if binding.Validator == nil {
		return nil
	}
	return binding.Validator.ValidateStruct(obj)
}
