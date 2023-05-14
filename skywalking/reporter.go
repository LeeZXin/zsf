package skywalking

import (
	"github.com/LeeZXin/zsf/appinfo"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/property/loader"
	"github.com/LeeZXin/zsf/quit"
	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
)

var (
	Tracer *go2sky.Tracer
)

// skywalking初始化失败 不影响服务启动

func init() {
	enableSw := property.GetBool("skywalking.enabled")
	if !enableSw {
		return
	}

	serverAddr := property.GetString("skywalking.serverAddr")
	if serverAddr == "" {
		logger.Logger.Error("empty skywalking serverAddr")
		return
	}

	maxSendQueueSize := 1024
	if property.GetInt("skywalking.maxSendQueueSize") > 0 {
		maxSendQueueSize = property.GetInt("skywalking.maxSendQueueSize")
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
	if property.GetFloat64("skywalking.samplerRate") > 0 {
		samplerRate = property.GetFloat64("skywalking.samplerRate")
	}
	tracer, err := go2sky.NewTracer(appinfo.GetApplicationName(),
		go2sky.WithReporter(grpcReporter),
		go2sky.WithSampler(samplerRate),
	)

	if err != nil {
		grpcReporter.Close()
		logger.Logger.Error(err)
		return
	}

	//动态调整采样率
	loader.OnKeyChange("skywalking.samplerRate", func() {
		rate := property.GetFloat64("skywalking.samplerRate")
		logger.Logger.Info("skywalking.samplerRate changed:", rate)
		go2sky.NewDynamicSampler(rate, tracer)
	})

	Tracer = tracer
	logger.Logger.Info("skywalking start tracer:", serverAddr)
	quit.AddShutdownHook(func() {
		grpcReporter.Close()
	})
}
