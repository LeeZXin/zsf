package httpclient

import (
	"github.com/LeeZXin/zsf-utils/collections/hashmap"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/quit"
	"net/http"
)

// 带服务发现的httpClient封装
var (
	clientCache = hashmap.NewConcurrentHashMap[string, Client]()
)

type Invoker func(*http.Request) (*http.Response, error)

type Interceptor func(*http.Request, Invoker) (*http.Response, error)

func init() {
	//注册三个拦截器
	RegisterInterceptors(
		headerInterceptor(),
		promInterceptor(),
	)
	//关闭所有的连接
	quit.AddShutdownHook(func() {
		clientCache.Range(func(_ string, client Client) {
			client.Close()
		})
	})
}

// Dial 获取服务的client
func Dial(serviceName string) Client {
	ret, _, _ := clientCache.GetOrPutWithLoader(serviceName, func() (Client, error) {
		return &clientImpl{
			ServiceName:  serviceName,
			Interceptors: getInterceptors(),
			httpClient:   httputil.NewRetryableHttp2Client(),
		}, nil
	})
	return ret
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
