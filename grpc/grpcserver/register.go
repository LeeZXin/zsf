package grpcserver

import (
	"github.com/LeeZXin/zsf/property/static"
	"google.golang.org/grpc"
	"sync"
)

type RegisterServiceFunc func(server *grpc.Server)

var (
	registerFuncList = make([]RegisterServiceFunc, 0)
	registerFuncMu   = sync.Mutex{}
)

var (
	unaryInterceptors  = make([]grpc.UnaryServerInterceptor, 0)
	unaryInterceptorMu = sync.Mutex{}

	streamInterceptors  = make([]grpc.StreamServerInterceptor, 0)
	streamInterceptorMu = sync.Mutex{}
)

func init() {
	if static.GetBool("application.disableMicro") {
		AppendUnaryInterceptors(
			logErrorUnaryInterceptor(),
		)
		AppendStreamInterceptors(
			logErrorStreamInterceptor(),
		)
	} else {
		AppendUnaryInterceptors(
			headerUnaryInterceptor(),
			logErrorUnaryInterceptor(),
			prometheusUnaryInterceptor(),
			skywalkingUnaryInterceptor(),
		)
		AppendStreamInterceptors(
			logErrorStreamInterceptor(),
			headerStreamInterceptor(),
			prometheusStreamInterceptor(),
			skywalkingStreamInterceptor(),
		)
	}
}

func AppendRegisterRouterFunc(f ...RegisterServiceFunc) {
	if len(f) == 0 {
		return
	}
	registerFuncMu.Lock()
	defer registerFuncMu.Unlock()
	registerFuncList = append(registerFuncList, f...)
}

func getRegisterFuncList() []RegisterServiceFunc {
	registerFuncMu.Lock()
	defer registerFuncMu.Unlock()
	return registerFuncList[:]
}

func AppendUnaryInterceptors(f ...grpc.UnaryServerInterceptor) {
	if len(f) == 0 {
		return
	}
	unaryInterceptorMu.Lock()
	defer unaryInterceptorMu.Unlock()
	unaryInterceptors = append(unaryInterceptors, f...)
}

func getUnaryInterceptors() []grpc.UnaryServerInterceptor {
	unaryInterceptorMu.Lock()
	defer unaryInterceptorMu.Unlock()
	return unaryInterceptors[:]
}

func AppendStreamInterceptors(f ...grpc.StreamServerInterceptor) {
	if len(f) == 0 {
		return
	}
	streamInterceptorMu.Lock()
	defer streamInterceptorMu.Unlock()
	streamInterceptors = append(streamInterceptors, f...)
}

func getStreamInterceptors() []grpc.StreamServerInterceptor {
	streamInterceptorMu.Lock()
	defer streamInterceptorMu.Unlock()
	return streamInterceptors[:]
}
