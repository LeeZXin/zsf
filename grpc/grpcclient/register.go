package grpcclient

import (
	"google.golang.org/grpc"
	"sync"
)

var (
	//全局拦截器
	unaryInterceptors = make([]grpc.UnaryClientInterceptor, 0)
	//全局拦截器
	streamInterceptors = make([]grpc.StreamClientInterceptor, 0)
	//锁
	unaryInterceptorMu = sync.Mutex{}
	//
	streamInterceptorMu = sync.Mutex{}
)

func AppendUnaryInterceptors(f ...grpc.UnaryClientInterceptor) {
	if len(f) == 0 {
		return
	}
	unaryInterceptorMu.Lock()
	defer unaryInterceptorMu.Unlock()
	unaryInterceptors = append(unaryInterceptors, f...)
}

func getUnaryInterceptors() []grpc.UnaryClientInterceptor {
	unaryInterceptorMu.Lock()
	defer unaryInterceptorMu.Unlock()
	return unaryInterceptors[:]
}

func AppendStreamInterceptors(f ...grpc.StreamClientInterceptor) {
	if len(f) == 0 {
		return
	}
	streamInterceptorMu.Lock()
	defer streamInterceptorMu.Unlock()
	streamInterceptors = append(streamInterceptors, f...)
}

func getStreamInterceptors() []grpc.StreamClientInterceptor {
	streamInterceptorMu.Lock()
	defer streamInterceptorMu.Unlock()
	return streamInterceptors[:]
}
