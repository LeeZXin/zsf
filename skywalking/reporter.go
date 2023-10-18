package skywalking

import (
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property/dynamic"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/quit"
	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
)

var (
	Tracer *go2sky.Tracer
)

// skywalking初始化失败 不影响服务启动

func init() {
	enableSw := static.GetBool("skywalking.enabled")
	if !enableSw {
		return
	}

	serverAddr := static.GetString("skywalking.serverAddr")
	if serverAddr == "" {
		logger.Logger.Error("empty skywalking serverAddr")
		return
	}

	maxSendQueueSize := 1024
	if static.GetInt("skywalking.maxSendQueueSize") > 0 {
		maxSendQueueSize = static.GetInt("skywalking.maxSendQueueSize")
	}

	grpcReporter, err := reporter.NewGRPCReporter(
		serverAddr,
		reporter.WithLog(logger.Logger),
		reporter.WithMaxSendQueueSize(maxSendQueueSize),
	)
	if err != nil {
		logger.Logger.Error(err)
		return
	}

	samplerRate := 0.6
	if static.GetFloat64("skywalking.samplerRate") > 0 {
		samplerRate = static.GetFloat64("skywalking.samplerRate")
	}
	tracer, err := go2sky.NewTracer(common.GetApplicationName(),
		go2sky.WithReporter(grpcReporter),
		go2sky.WithSampler(samplerRate),
	)

	if err != nil {
		grpcReporter.Close()
		logger.Logger.Error(err)
		return
	}

	//动态调整采样率
	dynamic.OnKeyChange("skywalking.samplerRate", func() {
		rate := static.GetFloat64("skywalking.samplerRate")
		logger.Logger.Info("skywalking.samplerRate changed:", rate)
		go2sky.NewDynamicSampler(rate, tracer)
	})

	Tracer = tracer
	logger.Logger.Info("skywalking start tracer:", serverAddr)
	quit.AddShutdownHook(func() {
		grpcReporter.Close()
	})
}
