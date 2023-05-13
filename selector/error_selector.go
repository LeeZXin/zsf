package selector

// ErrorSelector 配置错误选择器
type ErrorSelector struct {
	Err error
}

func (*ErrorSelector) Init() error {
	return nil
}

func (e *ErrorSelector) Select(key ...string) (*Node, error) {
	return nil, e.Err
}
