package httpclient

import (
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/rpcheader"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func headerInterceptor() Interceptor {
	return func(request *http.Request, invoker Invoker) (*http.Response, error) {
		headers := rpcheader.GetHeaders(request.Context())
		for k, v := range headers {
			if strings.HasPrefix(k, rpcheader.Prefix) {
				request.Header.Set(k, v)
			}
		}
		// 塞source信息
		request.Header.Set(rpcheader.Source, common.GetApplicationName())
		return invoker(request)
	}
}

func promInterceptor() Interceptor {
	return func(request *http.Request, invoker Invoker) (*http.Response, error) {
		begin := time.Now()
		response, err := invoker(request)
		if err == nil {
			target := request.Header.Get(rpcheader.Target)
			if target != "" {
				prom.HttpClientRequestTotal.
					WithLabelValues(target, request.URL.Path, strconv.Itoa(response.StatusCode)).
					Observe(float64(time.Since(begin).Milliseconds()))
			}
		}
		return response, err
	}
}
