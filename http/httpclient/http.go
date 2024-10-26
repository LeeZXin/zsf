package httpclient

import (
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/quit"
	"net/http"
	"sync"
)

// 带服务发现的httpClient封装
var (
	clientCache = make(map[string]Client)
	cacheMu     = sync.Mutex{}
)

type Invoker func(*http.Request) (*http.Response, error)

type Interceptor func(*http.Request, Invoker) (*http.Response, error)

func init() {
	//注册拦截器
	RegisterInterceptors(
		promInterceptor(),
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
	client = &clientImpl{
		ServiceName:  serviceName,
		Interceptors: getInterceptors(),
		httpClient:   httputil.NewHttp2Client(),
	}
	clientCache[serviceName] = client
	return client
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
