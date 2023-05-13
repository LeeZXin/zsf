package client

import (
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/quit"
	"github.com/LeeZXin/zsf/selector"
	"net/http"
	"sync"
)

// 带服务发现的httpClient封装
// 首次会加载服务ip数据，每10秒会尝试更新服务ip

var (
	clientCache = make(map[string]Client, 8)
	clientMu    = sync.Mutex{}

	interceptors   = make([]Interceptor, 0)
	interceptorsMu = sync.Mutex{}
)

type Invoker func(*http.Request) (*http.Response, error)

type Interceptor func(*http.Request, Invoker) (*http.Response, error)

func init() {
	//注册三个拦截器
	RegisterInterceptor(
		headerInterceptor(),
		promInterceptor(),
		skywalkingInterceptor(),
	)
	//关闭所有的连接
	quit.AddShutdownHook(func() {
		clientMu.Lock()
		defer clientMu.Unlock()
		for _, client := range clientCache {
			client.Close()
		}
	})
}

func Dial(serviceName string) Client {
	//双重校验
	client, ok := clientCache[serviceName]
	if ok {
		return client
	}
	clientMu.Lock()
	defer clientMu.Unlock()
	client, ok = clientCache[serviceName]
	if ok {
		return client
	}
	//初始化
	client = initClient(serviceName)
	clientCache[serviceName] = client
	return client
}

// initClient 初始化带有服务发现的http client
func initClient(serviceName string) Client {
	lbPolicyConfig := property.GetString("http.client.lbPolicy")
	var lbPolicy string
	_, ok := supportedLbPolicy[lbPolicyConfig]
	if ok {
		lbPolicy = lbPolicyConfig
	} else {
		lbPolicy = selector.RoundRobinPolicy
	}
	interceptorsMu.Lock()
	copyInterceptors := make([]Interceptor, len(interceptors))
	for i, interceptor := range interceptors {
		copyInterceptors[i] = interceptor
	}
	interceptorsMu.Unlock()
	c := &Impl{
		ServiceName:  serviceName,
		LbPolicy:     lbPolicy,
		Interceptors: copyInterceptors,
	}
	c.Init()
	return c
}

func RegisterInterceptor(is ...Interceptor) {
	if is == nil || len(is) == 0 {
		return
	}
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	interceptors = append(interceptors, is...)
}

type interceptorsWrapper struct {
	is []Interceptor
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
