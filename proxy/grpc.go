package proxy

import (
	"errors"
	grpcclient "github.com/LeeZXin/zsf/grpc/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"io"
	"sync"
)

var (
	// DefaultStreamDesc grpc流描述符 表示支持双向流
	DefaultStreamDesc = &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}
	NoMethodFoundErr = errors.New("not method found")
)

// Frame grpc请求体基本封装
type Frame struct {
	payload []byte
}

func (f *Frame) Reset() {}

func (f *Frame) String() string { return "" }

func (f *Frame) ProtoMessage() {}

func (f *Frame) Marshal() ([]byte, error) {
	return f.payload, nil
}

func (f *Frame) Unmarshal(d []byte) error {
	f.payload = d
	return nil
}

func (f *Frame) Copy() *Frame {
	return &Frame{
		payload: f.payload[:],
	}
}

// DoGrpcProxy 实际执行grpc反向代理函数
func DoGrpcProxy(rpcCtx *RpcContext) error {
	sourceStream := rpcCtx.Request().(grpc.ServerStream)
	// 读取方法名
	fullMethodName, ok := grpc.MethodFromServerStream(sourceStream)
	if !ok {
		return NoMethodFoundErr
	}
	var serviceName string
	if rpcCtx.trafficType == InBoundTraffic {
		// 获取目标target
		serviceName = rpcCtx.attachedHost
	} else {
		// 获取目标target
		serviceName = rpcCtx.TargetService()
	}
	// 服务发现寻找服务
	targetConn, err := grpcclient.Dial(serviceName)
	if err != nil {
		return err
	}
	ctx := sourceStream.Context()
	md := rpcCtx.Header()
	for key := range md {
		value := md.Get(key)
		ctx = metadata.AppendToOutgoingContext(ctx, key, value)
	}
	// 获取连接
	targetStream, err := grpc.NewClientStream(ctx, DefaultStreamDesc, targetConn, fullMethodName)
	if err != nil {
		return err
	}
	// 等待锁
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(2)
	// 两个流循环获取发送 知道读取到 io.EOF
	recvStream := func() error {
		defer waitGroup.Done()
		frame := Frame{}
		for {
			err1 := targetStream.RecvMsg(&frame)
			if err1 == io.EOF {
				return nil
			}
			if err1 != nil {
				return err1
			}
			err1 = sourceStream.SendMsg(&frame)
			if err1 != nil {
				return err1
			}
		}
	}
	sendStream := func() error {
		defer waitGroup.Done()
		frame := Frame{}
		for {
			err2 := sourceStream.RecvMsg(&frame)
			if err2 == io.EOF {
				return nil
			}
			if err2 != nil {
				return err2
			}
			err2 = targetStream.SendMsg(&frame)
			if err2 != nil {
				return err2
			}
		}
	}
	var (
		err3 error
		err4 error
	)
	go func() {
		err3 = sendStream()
	}()
	err4 = recvStream()
	waitGroup.Wait()
	if err3 != nil {
		return err3
	}
	if err4 != nil {
		return err4
	}
	return nil
}
