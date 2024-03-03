package apigw

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/services/discovery"
)

type hostSelector interface {
	Select(context.Context) (string, error)
}

type ipPortSelector struct {
	serviceName string
}

func (s *ipPortSelector) Select(ctx context.Context) (string, error) {
	server, err := discovery.ChooseServer(ctx, s.serviceName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", server.Host, server.Port), nil
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
