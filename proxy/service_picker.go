package proxy

import (
	"github.com/LeeZXin/zsf/rpc"
)

// TargetServiceNamePicker 目标服务选择器
type TargetServiceNamePicker func(RpcContext) (string, error)

func DefaultTargetServiceNamePicker(rpcContext RpcContext) (string, error) {
	header := rpcContext.Header()
	target := header.Get(rpc.Target)
	if target == "" {
		return "", TargetNotFoundErr
	}
	return target, nil
}
