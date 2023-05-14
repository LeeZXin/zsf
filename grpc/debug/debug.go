package debug

import (
	"github.com/LeeZXin/zsf/logger"
	"google.golang.org/grpc/grpclog"
)

// StartGrpcDebug 开启grpc debug模式
func StartGrpcDebug() {
	grpclog.SetLoggerV2(logger.Logger)
}
