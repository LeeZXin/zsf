package selector

import "context"

// ErrorSelector 配置错误选择器
type ErrorSelector struct {
	Err error
}

func (e *ErrorSelector) Select(ctx context.Context, key ...string) (node Node, err error) {
	err = e.Err
	return
}
