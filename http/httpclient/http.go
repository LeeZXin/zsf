package httpclient

import (
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/quit"
	"net/http"
	"sync"
)

// 带服务发现的httpClient封装
// 首次会加载服务ip数据，每10秒会尝试更新服务ip

var (
	clientCache = make(map[string]Client, 8)
	cacheMu     = sync.Mutex{}

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
		cacheMu.Lock()
		defer cacheMu.Unlock()
		for _, client := range clientCache {
			client.Close()
		}
	})
}

// Dial 获取服务的client
func Dial(serviceName string) Client {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	client, ok := clientCache[serviceName]
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
	lbPolicyConfig := static.GetString("http.client.lbPolicy")
	var lbPolicy string
	if supportedLbPolicy.Contains(lbPolicyConfig) {
		lbPolicy = lbPolicyConfig
	} else {
		lbPolicy = selector.RoundRobinPolicy
	}
	interceptorsMu.Lock()
	copyInterceptors := interceptors[:]
	interceptorsMu.Unlock()
	c := &clientImpl{
		ServiceName:  serviceName,
		LbPolicy:     lbPolicy,
		Interceptors: copyInterceptors,
	}
	c.init()
	return c
}

// RegisterInterceptor 注册一个client自定义拦截器
func RegisterInterceptor(is ...Interceptor) {
	if is == nil || len(is) == 0 {
		return
	}
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	interceptors = append(interceptors, is...)
}

// 拦截器wrapper 实现类似洋葱递归执行功能
type interceptorsWrapper struct {
	interceptorList []Interceptor
}

func (i *interceptorsWrapper) intercept(request *http.Request, invoker Invoker) (*http.Response, error) {
	if i.interceptorList == nil || len(i.interceptorList) == 0 {
		return invoker(request)
	}
	return i.recursive(0, request, invoker)
}

func (i *interceptorsWrapper) recursive(index int, request *http.Request, invoker Invoker) (*http.Response, error) {
	return i.interceptorList[index](request, func(request *http.Request) (*http.Response, error) {
		if index == len(i.interceptorList)-1 {
			return invoker(request)
		} else {
			return i.recursive(index+1, request, invoker)
		}
	})
}
