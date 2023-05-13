package selector

// SingleNodeSelector 单节点选择器 当节点只有一个时
type SingleNodeSelector struct {
	Node Node
}

func (s *SingleNodeSelector) Select(key ...string) (Node, error) {
	return s.Node, nil
}
