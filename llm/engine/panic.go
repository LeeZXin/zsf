package engine

import (
	"fmt"
)

// panicErr 封装 panic 信息与调用栈，实现 error 接口，
// Error 输出 panic 值与前缀堆栈，便于排查底层 SDK 崩溃。
type panicErr struct {
	info  any
	stack []byte
}

// Error 实现 error 接口。
func (p *panicErr) Error() string {
	return fmt.Sprintf("panic error: %v, \nstack: %s", p.info, string(p.stack))
}

// NewPanicErr 构造 panic 错误（panic 值与堆栈），供 panicSafeReader 等
// recover 路径把底层 panic 转成可返回的错误。
func NewPanicErr(info any, stack []byte) error {
	return &panicErr{
		info:  info,
		stack: stack,
	}
}
