package debug

import (
	"google.golang.org/grpc/grpclog"
	"zsf/logger"
)

//开启grpc debug模式

func StartGrpcDebug() {
	grpclog.SetLoggerV2(logger.Logger)
}
