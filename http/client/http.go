package httpclient

import (
	"net/http"
	"sync"
	"zsf/cache"
	"zsf/property"
	"zsf/quit"
	"zsf/selector"
)

// 带服务发现的httpClient封装
// 首次会加载服务ip数据，每10秒会尝试更新服务ip

var (
	clientCache                *cache.MapCache
	globalClientInterceptors   = make([]ClientInterceptor, 0)
	globalClientInterceptorsMu = sync.Mutex{}
)

type Invoker func(*http.Request) (*http.Response, error)

type ClientInterceptor func(*http.Request, Invoker) (*http.Response, error)

func init() {
	//注册两个拦截器
	RegisterGlobalClientInterceptor(headerInterceptor(), promInterceptor(), skywalkingInterceptor())
	//加载缓存
	clientCache = &cache.MapCache{
		SupplierWithKey: func(serviceName string) (any, error) {
			lbPolicyConfig := property.GetString("http.client.lbPolicy")
			var lbPolicy selector.LbPolicy
			policy, ok := supportedLbPolicy[lbPolicyConfig]
			if ok {
				lbPolicy = policy
			} else {
				lbPolicy = selector.RoundRobinPolicy
			}
			globalClientInterceptorsMu.Lock()
			is := make([]ClientInterceptor, len(globalClientInterceptors))
			for i, interceptor := range globalClientInterceptors {
				is[i] = interceptor
			}
			globalClientInterceptorsMu.Unlock()
			c := &ClientImpl{
				ServiceName:  serviceName,
				LbPolicy:     lbPolicy,
				Interceptors: is,
			}
			c.Init()
			return c, nil
		},
	}
	//关闭所有的连接
	quit.RegisterQuitFunc(func() {
		keys := clientCache.AllKeys()
		for _, key := range keys {
			dial, err := Dial(key)
			if err == nil {
				dial.Close()
			}
		}
	})
}

func Dial(serviceName string) (Client, error) {
	c, err := clientCache.Get(serviceName)
	if err != nil {
		return nil, err
	}
	return c.(Client), err
}

func RegisterGlobalClientInterceptor(is ...ClientInterceptor) {
	if is == nil || len(is) == 0 {
		return
	}
	globalClientInterceptorsMu.Lock()
	defer globalClientInterceptorsMu.Unlock()
	globalClientInterceptors = append(globalClientInterceptors, is...)
}

type interceptorsWrapper struct {
	is []ClientInterceptor
}

func (i *interceptorsWrapper) intercept(request *http.Request, invoker Invoker) (*http.Response, error) {
	if i.is == nil || len(i.is) == 0 {
		return invoker(request)
	}
	return i.recursive(0, request, invoker)
}

func (i *interceptorsWrapper) recursive(index int, request *http.Request, invoker Invoker) (*http.Response, error) {
	return i.is[index](request, func(request *http.Request) (*http.Response, error) {
		if index == len(i.is)-1 {
			return invoker(request)
		} else {
			return i.recursive(index+1, request, invoker)
		}
	})
}
