package proxy

import (
	"github.com/LeeZXin/zsf/rpc"
)

// ServiceNamePicker 目标服务选择器
type ServiceNamePicker func(RpcContext) (string, error)

func DefaultTargetServiceNamePicker(rpcContext RpcContext) (string, error) {
	header := rpcContext.Header()
	target := header.Get(rpc.Target)
	if target == "" {
		return "", TargetNotFoundErr
	}
	return target, nil
}

func DefaultSourceServiceNamePicker(rpcContext RpcContext) (string, error) {
	header := rpcContext.Header()
	target := header.Get(rpc.Source)
	if target == "" {
		return "", SourceNotFoundErr
	}
	return target, nil
}
