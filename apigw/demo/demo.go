package demo

import (
	"github.com/LeeZXin/zsf/apigw"
	"github.com/LeeZXin/zsf/selector"
	"github.com/gin-gonic/gin"
)

var (
	routers = &apigw.Routers{}
)

func init() {
	err := routers.AddRouter(apigw.RouterConfig{
		MatchType:      apigw.FullMatchType,
		Path:           "/index",
		Expr:           nil,
		ServiceName:    "my-runner-http",
		Targets:        nil,
		TargetType:     apigw.DiscoveryTargetType,
		TargetLbPolicy: selector.RoundRobinPolicy,
		RewriteType:    apigw.ReplaceAnyRewriteType,
		ReplacePath:    "/header",
	})
	if err != nil {
		panic(err)
	}

	err = routers.AddRouter(apigw.RouterConfig{
		MatchType:   apigw.FullMatchType,
		Path:        "/host",
		Expr:        nil,
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
		MockContent: &apigw.MockContent{
			ContentType: apigw.MockJsonType,
			StatusCode:  200,
			RespStr:     `{"code": 2001, "message": "success"}`,
		},
	})
	if err != nil {
		panic(err)
	}

	err = routers.AddRouter(apigw.RouterConfig{
		MatchType:  apigw.FullMatchType,
		Path:       "/mock2",
		TargetType: apigw.MockTargetType,
		MockContent: &apigw.MockContent{
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
