// Package ginutil 提供基于 Gin 框架的 HTTP 工具函数。
// 包含请求参数绑定、响应封装、文件上传、分页请求/响应结构体等常用功能。
package ginutil

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/constants"
	"github.com/LeeZXin/zsf/http/bizerr"

	"github.com/gin-gonic/gin"
)

var (
	// DefaultSuccessResp 默认的成功响应，code 为 0
	DefaultSuccessResp = BaseResp{
		Code: 0,
	}
)

// BaseResp 基础 HTTP 响应结构体
type BaseResp struct {
	Code    int    `json:"code"`              // 业务状态码，0 表示成功，非 0 表示失败
	Message string `json:"message,omitempty"` // 错误消息，仅在失败时返回
}

// IsSuccess 判断响应是否表示业务成功（code == 0）
func (r *BaseResp) IsSuccess() bool {
	return r.Code == 0
}

// DataResp 带数据的通用响应结构体（泛型）
type DataResp[T any] struct {
	BaseResp
	Data T `json:"data"` // 响应数据
}

// CursorReq 游标分页请求结构体
type CursorReq struct {
	Before   int64 `json:"before"`   // 上一页游标（上一页最后一条记录的 ID）
	After    int64 `json:"after"`    // 下一页游标（当前页最后一条记录的 ID）
	PageSize int   `json:"pageSize"` // 每页大小（1-100）
}

// IsValid 校验游标分页请求参数是否合法
func (r *CursorReq) IsValid() bool {
	return r.Before >= 0 && r.After >= 0 && r.PageSize >= 1 && r.PageSize <= 100
}

// CursorResp 游标分页响应结构体（泛型）
type CursorResp[T any] struct {
	Data    []T   `json:"data"`    // 数据列表
	Before  int64 `json:"before"`  // 上一页游标
	After   int64 `json:"after"`   // 下一页游标
	HasNext bool  `json:"hasNext"` // 是否有下一页
	HasPrev bool  `json:"hasPrev"` // 是否有上一页
}

// Page2Req 传统分页请求结构体
type Page2Req struct {
	PageNum  int `json:"pageNum"`  // 页码（从 1 开始）
	PageSize int `json:"pageSize"` // 每页大小（1-100）
}

// IsValid 校验分页请求参数是否合法
func (r *Page2Req) IsValid() bool {
	return r.PageNum >= 1 && r.PageSize >= 1 && r.PageSize <= 100
}

// Page2Resp 传统分页响应结构体（泛型）
type Page2Resp[T any] struct {
	Data    []T   `json:"data"`    // 数据列表
	PageNum int   `json:"pageNum"` // 当前页码
	Total   int64 `json:"total"`   // 总记录数
}

// HandleErr 处理错误并写入 HTTP 响应。
// 如果是 *bizerr.Err 类型的业务错误，调用 Error 写入 JSON 响应；
// 否则返回 500 状态码。
func HandleErr(err error, c *gin.Context) {
	if err != nil {
		var berr *bizerr.Err
		ok := errors.As(err, &berr)
		if !ok {
			c.String(http.StatusInternalServerError, "")
		} else {
			Error(berr, c)
		}
	}
}

// handleBindErr 处理参数绑定错误。
// 如果是 MaxBytesError（请求体过大），返回 413 状态码；否则返回 400。
func handleBindErr(err error, c *gin.Context) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
	} else {
		c.AbortWithStatus(http.StatusBadRequest)
	}
}

// ShouldBind 绑定请求参数（根据 Content-Type 自动选择绑定方式）。
// 绑定失败时自动返回 400 或 413 状态码。
// 返回: true 表示绑定成功，false 表示绑定失败
func ShouldBind(obj any, c *gin.Context) bool {
	err := c.ShouldBind(obj)
	if err != nil {
		handleBindErr(err, c)
		return false
	}
	return true
}

// ShouldBindJSON 绑定 JSON 请求体到结构体。
// 绑定失败时自动返回 400 或 413 状态码。
// 返回: true 表示绑定成功，false 表示绑定失败
func ShouldBindJSON(obj any, c *gin.Context) bool {
	err := c.ShouldBindJSON(obj)
	if err != nil {
		handleBindErr(err, c)
		return false
	}
	return true
}

// ShouldBindQuery 绑定 URL 查询参数到结构体。
// 绑定失败时自动返回 400 状态码。
// 返回: true 表示绑定成功，false 表示绑定失败
func ShouldBindQuery(obj any, c *gin.Context) bool {
	err := BindQuery(c, obj)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return false
	}
	return true
}

// ShouldBindMultipartForm 绑定 multipart/form-data 或 x-www-form-urlencoded 表单到结构体。
// 绑定失败时自动返回 400 状态码。
// 返回: true 表示绑定成功，false 表示绑定失败
func ShouldBindMultipartForm(obj any, c *gin.Context) bool {
	err := BindMultipartForm(c, obj)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return false
	}
	return true
}

// GetClientIp 获取客户端 IP 地址。
// 如果是 IPv6 环回地址(::1)，则转换为 127.0.0.1。
func GetClientIp(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "::1" {
		return "127.0.0.1"
	}
	return ip
}

// GetFile 从请求中获取上传文件的读取流。
// 如果是表单提交，读取第一个上传文件；否则直接返回请求体。
// 返回: 文件读取流和错误信息
func GetFile(c *gin.Context) (io.ReadCloser, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, err
		}
		if c.Request.MultipartForm.File == nil {
			return nil, http.ErrMissingFile
		}
		for _, files := range c.Request.MultipartForm.File {
			if len(files) > 0 {
				return files[0].Open()
			}
		}
		return nil, http.ErrMissingFile
	}
	return c.Request.Body, nil
}

// GetFormFile 从请求中获取第一个上传文件的 FileHeader。
// 返回: 文件头信息和错误信息
func GetFormFile(c *gin.Context) (*multipart.FileHeader, error) {
	if c.Request.MultipartForm != nil {
		for _, files := range c.Request.MultipartForm.File {
			if len(files) > 0 {
				return files[0], nil
			}
		}
		return nil, http.ErrMissingFile
	}
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, err
		}
		if c.Request.MultipartForm.File == nil {
			return nil, http.ErrMissingFile
		}
		for _, files := range c.Request.MultipartForm.File {
			if len(files) > 0 {
				return files[0], nil
			}
		}
	}
	return nil, http.ErrMissingFile
}

// DefaultSuccess 返回默认的成功 JSON 响应（code=0，无数据）。
func DefaultSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, DefaultSuccessResp)
}

// Success 直接返回任意数据作为 JSON 响应。
func Success(data any, c *gin.Context) {
	c.JSON(http.StatusOK, data)
}

// DataSuccess 返回带数据的 JSON 成功响应，格式为 {code:0, data: T}。
func DataSuccess[T any](data T, c *gin.Context) {
	c.JSON(http.StatusOK, DataResp[T]{
		BaseResp: DefaultSuccessResp,
		Data:     data,
	})
}

// Page2Success 返回传统分页格式的成功 JSON 响应。
// data: 当前页数据列表; total: 总记录数
func Page2Success[T any](data []T, total int64, c *gin.Context) {
	c.JSON(http.StatusOK, DataResp[Page2Resp[T]]{
		BaseResp: DefaultSuccessResp,
		Data: Page2Resp[T]{
			Total: total,
			Data:  data,
		},
	})
}

// Error 返回业务错误的 JSON 响应。
// 如果错误标记为内部错误（Internal=true），会在 Gin 上下文中设置内部错误标记，供熔断器使用。
func Error(err *bizerr.Err, c *gin.Context) {
	if err.Internal {
		c.Set(constants.HttpInternalErr, true)
	}
	c.JSON(http.StatusOK, BaseResp{
		Code:    err.Code,
		Message: err.Message,
	})
}

// GetUserAgent 获取请求的 User-Agent 头信息。
func GetUserAgent(c *gin.Context) string {
	return c.GetHeader("User-Agent")
}

// GetReferer 获取请求的 Referer 头信息。
func GetReferer(c *gin.Context) string {
	return c.GetHeader("Referer")
}

// NoCacheHeader 设置 HTTP 响应头，禁用浏览器缓存。
func NoCacheHeader(c *gin.Context) {
	c.Header("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	c.Header("Pragma", "no-cache")
	c.Header("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

// CacheHeader 设置 HTTP 响应头，启用浏览器缓存并指定缓存时长。
// duration: 缓存过期时间
func CacheHeader(c *gin.Context, duration time.Duration) {
	now := time.Now().UTC()
	expires := now.Add(duration)
	c.Header("Date", now.Format(http.TimeFormat))
	c.Header("Expires", expires.Format(http.TimeFormat))
	c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", int(duration.Seconds())))
}

// BearerToken 从 Authorization 头解析 Bearer token。
func BearerToken(c *gin.Context) (string, bool) {
	auth := c.GetHeader("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return token, token != ""
}
