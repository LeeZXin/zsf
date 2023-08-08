package selector

import "context"

// ErrorSelector 配置错误选择器
type ErrorSelector[T any] struct {
	Err error
}

func (e *ErrorSelector[T]) Select(ctx context.Context, key ...string) (node Node[T], err error) {
	err = e.Err
	return
}
