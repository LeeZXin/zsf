package proxy

// 拦截器
// 可执行限流、鉴权等拦截器
// 默认不实现任何拦截器

type Invoker func(*RpcContext) error

type Interceptor func(*RpcContext, Invoker) error

type interceptorsWrapper struct {
	interceptorList []Interceptor
}

func (i *interceptorsWrapper) intercept(rpcContext *RpcContext, invoker Invoker) error {
	if i.interceptorList == nil || len(i.interceptorList) == 0 {
		return invoker(rpcContext)
	}
	return i.recursive(0, rpcContext, invoker)
}

func (i *interceptorsWrapper) recursive(index int, rpcContext *RpcContext, invoker Invoker) error {
	return i.interceptorList[index](rpcContext, func(rpcContext *RpcContext) error {
		if index == len(i.interceptorList)-1 {
			return invoker(rpcContext)
		} else {
			return i.recursive(index+1, rpcContext, invoker)
		}
	})
}
