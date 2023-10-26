package demo

import (
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf/apigw"
	"github.com/LeeZXin/zsf/apigw/hexpr"

	"github.com/gin-gonic/gin"
)

var (
	routers = apigw.NewRouters(httputil.NewRetryableHttpClient())
)

func init() {
	err := routers.AddRouter(apigw.RouterConfig{
		MatchType:      apigw.FullMatchType,
		Path:           "/index",
		Expr:           hexpr.PlainInfo{},
		ServiceName:    "my-runner-http",
		Targets:        nil,
		TargetType:     apigw.DiscoveryTargetType,
		TargetLbPolicy: selector.RoundRobinPolicy,
		RewriteType:    apigw.ReplaceAnyRewriteType,
		ReplacePath:    "/header",
		NeedAuth:       true,
		AuthConfig: apigw.AuthConfig{
			Id:      "1",
			UriType: apigw.DiscoveryUriType,
			Uri: apigw.AuthUri{
				DiscoveryTarget: "my-runner-grpc",
				Path:            "/auth",
				Timeout:         10,
			},
			Parameters: []apigw.AuthParameter{
				{
					TargetName:     "u",
					TargetLocation: apigw.QueryLocation,
					SourceName:     "c",
					SourceLocation: apigw.QueryLocation,
				}, {
					TargetName:     "uu",
					TargetLocation: apigw.QueryLocation,
					SourceName:     "uu",
					SourceLocation: apigw.QueryLocation,
				}, {
					TargetName:     "uuu",
					TargetLocation: apigw.HeaderLocation,
					SourceName:     "ccc",
					SourceLocation: apigw.QueryLocation,
				}, {
					TargetName:     "uuuu",
					TargetLocation: apigw.HeaderLocation,
					SourceName:     "Cookie",
					SourceLocation: apigw.HeaderLocation,
				},
			},
			ErrorMessage:    "fucku",
			ErrorStatusCode: 401,
			PassThroughHeaderList: []string{
				"z-token",
			},
		},
	})
	if err != nil {
		panic(err)
	}

	err = routers.AddRouter(apigw.RouterConfig{
		MatchType:   apigw.FullMatchType,
		Path:        "/host",
		Expr:        hexpr.PlainInfo{},
		ServiceName: "",
		Targets: []apigw.Target{
			{
				Weight: 1,
				Target: "http://www.baidu1.com",
			},
			{
				Weight: 1,
				Target: "http://www.baidu2.com",
			},
		},
		TargetType:     apigw.DomainTargetType,
		TargetLbPolicy: selector.RoundRobinPolicy,
		RewriteType:    apigw.CopyFullPathRewriteType,
		ReplacePath:    "",
	})
	if err != nil {
		panic(err)
	}

	err = routers.AddRouter(apigw.RouterConfig{
		MatchType:  apigw.FullMatchType,
		Path:       "/mock1",
		TargetType: apigw.MockTargetType,
		MockContent: apigw.MockContent{
			ContentType: apigw.MockJsonType,
			StatusCode:  200,
			RespStr:     `{"code": 2001, "message": "success"}`,
		},
	})
	if err != nil {
		panic(err)
	}

	err = routers.AddRouter(apigw.RouterConfig{
		MatchType:  apigw.PrefixMatchType,
		Path:       "/mock2",
		TargetType: apigw.MockTargetType,
		MockContent: apigw.MockContent{
			ContentType: apigw.MockStringType,
			StatusCode:  400,
			RespStr:     `666666iiiii`,
		},
	})
	if err != nil {
		panic(err)
	}
}

func Proxy() gin.HandlerFunc {
	return func(c *gin.Context) {
		transport, ok := routers.FindTransport(c)
		if ok {
			transport.Transport(c)
			c.Abort()
		} else {
			c.Next()
		}
	}
}
