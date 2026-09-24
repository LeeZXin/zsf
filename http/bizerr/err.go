// Package bizerr 定义业务错误（BizErr）类型及其构造/识别工具。
//
// 职责边界：业务错误是与 HTTP 层解耦的错误载体，由业务代码主动构造并返回，
// 最终经 ginutil.HandleErr 转换为 {code, message} JSON 响应（HTTP 状态码仍为 200，
// 成败由 code 字段区分）。与之相对，非业务错误会被视为系统错误，返回 500。
//
// 约定：
//   - code 为业务状态码，0 表示成功，非 0 表示失败；各服务自行规划错误码段
//   - Internal 标记内部错误：置位后经 ginutil.Error 写入 gin context，
//     供 Sentinel 熔断器将此类请求计入错误率（见 http/server 的 SentinelFilter）
package bizerr

import (
	"errors"
	"fmt"
)

// Err 业务错误结构
// 包含错误码、错误消息和内部错误标识
type Err struct {
	Code     int    `json:"code"`     // 错误码
	Message  string `json:"message"`  // 错误消息
	Internal bool   `json:"internal"` // 是否为内部错误
}

// Error 实现error接口，返回错误描述
// 返回: 格式化后的错误字符串
func (e *Err) Error() string {
	return fmt.Sprintf("ErrCode: %d, Message: %s", e.Code, e.Message)
}

// NewBizErr 创建业务错误
// 参数: code - 错误码; format - 错误消息格式; args - 格式化参数
// 返回: 业务错误指针
func NewBizErr(code int, format string, args ...any) *Err {
	return &Err{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// IsBizErr 判断错误是否为业务错误
// 参数: err - 原始错误
// 返回: true表示是业务错误，false表示不是
func IsBizErr(err error) bool {
	if err == nil {
		return false
	}
	var berr *Err
	return errors.As(err, &berr)
}
