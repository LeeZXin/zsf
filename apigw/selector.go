package apigw

import (
	"context"
	"github.com/LeeZXin/zsf-utils/selector"
)

type Selector interface {
	Select(context.Context) (string, error)
}

type SelectorWrapper struct {
	selector.Selector[string]
}

func (s *SelectorWrapper) Select(context.Context) (string, error) {
	ret, err := s.Selector.Select()
	if err != nil {
		return "", err
	}
	return ret.Data, nil
}
