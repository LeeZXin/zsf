// Package threadutil 提供安全的函数执行工具：捕获 panic 并转换为带调用栈的错误返回，
// 避免 goroutine 中的 panic 导致整个进程崩溃。
package threadutil

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
)

// RunSafe 在调用方 goroutine 中同步执行 fn（不新开 goroutine）。
// fn 正常返回时返回 nil；fn 触发 panic 时捕获并返回附带调用栈的 error，不向上抛 panic。
// cleanup 中的函数无论 fn 是否 panic 都会按顺序执行（通常用于资源清理）。
// 注意：panic 中的值若不是 error 或 string，会被包装为 "unknown errors"。
func RunSafe(fn func(), cleanup ...func()) (err error) {
	defer func() {
		for _, c := range cleanup {
			if c != nil {
				c()
			}
		}
		if e := recover(); e != nil {
			var wrapped error
			switch v := e.(type) {
			case error:
				wrapped = v
			case string:
				wrapped = errors.New(v)
			default:
				wrapped = errors.New("unknown errors")
			}
			err = newRuntimeErr(wrapped, 10, 4)
		}
	}()
	fn()
	return nil
}

func prettyErrCallerTrace(depth int, starts ...int) string {
	stack := make([]string, 0, depth)
	start := 0
	if len(starts) > 0 {
		start = starts[0]
	}
	for i := start; i < depth+start; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		stack = append(stack, "		"+file+":"+strconv.Itoa(line))
	}
	return strings.Join(stack, "\n")
}

type runtimeErr struct {
	err        error
	callTraces string
}

func (r *runtimeErr) Error() string {
	if r.err == nil {
		return ""
	}
	return r.err.Error() + "\n" + r.callTraces
}

func newRuntimeErr(err error, depth int, skip ...int) error {
	return &runtimeErr{
		err:        err,
		callTraces: prettyErrCallerTrace(depth, skip...),
	}
}
