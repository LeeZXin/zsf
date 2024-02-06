package apigw

import (
	"context"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/discovery"
)

type hostSelector interface {
	Select(context.Context) (string, error)
}

type ipPortSelector struct {
	serviceName string
}

func (s *ipPortSelector) Select(ctx context.Context) (string, error) {
	return discovery.PickOneHost(ctx, s.serviceName)
}

type emptySelector struct {
}

func (s *emptySelector) Select(context.Context) (string, error) {
	return "", nil
}

type selectorWrapper struct {
	selector.Selector[string]
}

func (s *selectorWrapper) Select(context.Context) (string, error) {
	ret, err := s.Selector.Select()
	if err != nil {
		return "", err
	}
	return ret.Data, nil
}
