package main

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/application"
	grpcclient "github.com/LeeZXin/zsf/grpc/client"
	hello "github.com/LeeZXin/zsf/grpc/proto"
	httpclient "github.com/LeeZXin/zsf/http/client"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/property/loader"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"net/http"
)

type Demo struct {
}

func (*Demo) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	dial := httpclient.Dial("my-runner-http")
	err := dial.Get(ctx, "/header", nil)
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
	dial := httpclient.Dial("my-runner-http")
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
	err := dial.Get(s.Context(), "/header", &resp, httpclient.WithHeader(map[string]string{
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
	application.RegisterGrpcService(func(server *grpc.Server) {
		hello.RegisterHelloServiceServer(server, &Demo{})
	})
	application.SetGrpcStreamServerInterceptors([]grpc.StreamServerInterceptor{})
	application.RegisterHttpRouter(func(e *gin.Engine) {
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
	loader.OnKeyChange("kkk", func() {
		fmt.Println("hhhhh")
	})
	application.Run()
}
