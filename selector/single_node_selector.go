package selector

import "context"

// SingleNodeSelector 单节点选择器 当节点只有一个时
type SingleNodeSelector[T any] struct {
	Node Node[T]
}

func (s *SingleNodeSelector[T]) Select(ctx context.Context, key ...string) (Node[T], error) {
	return s.Node, nil
}
