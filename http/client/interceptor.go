package httpclient

import (
	"fmt"
	"github.com/LeeZXin/zsf/app"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/skywalking"
	"github.com/SkyAPM/go2sky"
	"net/http"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	"strconv"
	"time"
)

func headerInterceptor() ClientInterceptor {
	return func(request *http.Request, invoker Invoker) (*http.Response, error) {
		headers := rpc.GetHeaders(request.Context())
		for k, v := range headers {
			request.Header.Set(k, v)
		}
		request.Header.Set(rpc.Source, app.ApplicationName)
		return invoker(request)
	}
}

func promInterceptor() ClientInterceptor {
	return func(request *http.Request, invoker Invoker) (*http.Response, error) {
		begin := time.Now()
		defer prom.HttpClientRequestTotal.WithLabelValues("http://" + request.Host + request.URL.Path).Observe(float64(time.Since(begin).Milliseconds()))
		return invoker(request)
	}
}

func skywalkingInterceptor() ClientInterceptor {
	return func(request *http.Request, invoker Invoker) (*http.Response, error) {
		if skywalking.Tracer == nil {
			return invoker(request)
		}
		operationName := fmt.Sprintf("%s %s", request.Method, request.URL)
		ctx := request.Context()
		target := request.Header.Get(rpc.Target)
		if target == "" {
			target = "#"
		}
		span, err := skywalking.Tracer.CreateExitSpan(ctx, operationName, target, func(key, value string) error {
			request.Header.Set(rpc.PrefixForSw+key, value)
			return nil
		})
		if err != nil {
			return invoker(request)
		}
		defer span.End()
		span.SetComponent(skywalking.ComponentIDGOHttpClient)
		span.Tag(go2sky.TagHTTPMethod, request.Method)
		span.Tag(go2sky.TagURL, request.URL.String())
		span.Tag(skywalking.TagRpcScheme, skywalking.TagHttpScheme)
		span.SetSpanLayer(agentv3.SpanLayer_Http)
		resp, err := invoker(request)
		if err != nil {
			span.Error(time.Now(), err.Error())
		} else {
			span.Tag(go2sky.TagStatusCode, strconv.Itoa(resp.StatusCode))
			if resp.StatusCode >= http.StatusBadRequest {
				span.Error(time.Now(), "Errors on handling client")
			}
		}
		return resp, nil
	}
}
