package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"net/http"
	grpcclient "zsf/grpc/client"
	hello "zsf/grpc/proto"
	httpclient "zsf/http/client"
	"zsf/logger"
	"zsf/property"
	"zsf/starter"
)

type Demo struct {
}

func (*Demo) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	dial, err := httpclient.Dial("my-runner-http")
	if err != nil {
		return nil, err
	}

	err = dial.Get(ctx, "/header", nil)
	if err != nil {
		return nil, err
	}
	err = dial.Get(ctx, "/header", nil)
	if err != nil {
		return nil, err
	}
	err = dial.Get(ctx, "/header", nil)
	if err != nil {
		return nil, err
	}
	return &hello.HelloResp{
		Code:    1,
		Message: "xx",
	}, nil
}
func (*Demo) HelloStream(req *hello.HelloReq, s hello.HelloService_HelloStreamServer) error {
	return nil
}

func (*Demo) HelloStreamStream(s hello.HelloService_HelloStreamStreamServer) error {
	logger.Logger.WithContext(s.Context()).Info("stream")
	dial, err := httpclient.Dial("my-runner-http")
	resp := make(map[string]any)
	for i := 0; i < 5; i++ {
		req := hello.HelloReq{}
		err := s.RecvMsg(&req)
		if err == nil {
			logger.Logger.WithContext(s.Context()).Info(req)
		} else {
			logger.Logger.WithContext(s.Context()).Error(err)
		}
	}
	logger.Logger.WithContext(s.Context()).Info("cacn:", s.Context().Err())
	err = dial.Get(s.Context(), "/header", &resp, httpclient.WithHeader(map[string]string{
		"nnn-xx": "xxxxggg",
	}))
	if err != nil {
		logger.Logger.WithContext(s.Context()).Error(err)
	} else {
		logger.Logger.WithContext(s.Context()).Info(resp)
	}
	return nil
}

func main() {
	starter.RegisterGrpcService(func(server *grpc.Server) {
		hello.RegisterHelloServiceServer(server, &Demo{})
	})
	starter.SetGrpcStreamServerInterceptors([]grpc.StreamServerInterceptor{})
	starter.RegisterHttpRouter(func(e *gin.Engine) {
		e.Any("/header", func(c *gin.Context) {
			logger.Logger.WithContext(c.Request.Context()).Info("header test", c.Request.Header)
			c.JSON(http.StatusOK, gin.H{
				"code":    1,
				"message": "xxx",
			})
		})
		e.Any("/index", func(c *gin.Context) {
			dial, err := grpcclient.Dial("my-runner-grpc")
			if err != nil {
				logger.Logger.Error(err)
			} else {
				client := hello.NewHelloServiceClient(dial)
				resp, err := client.Hello(c.Request.Context(), &hello.HelloReq{
					Code: 999,
				})
				if err != nil {
					logger.Logger.Error(err)
				} else {
					logger.Logger.Info(resp)
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"code":    1,
				"message": property.GetString("hhh"),
			})
		})
	})
	starter.Run()
}
