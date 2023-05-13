package skywalking

import (
	"github.com/LeeZXin/zsf/app"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/property_loader"
	"github.com/LeeZXin/zsf/quit"
	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
)

var (
	Tracer *go2sky.Tracer
)

// skywalking初始化失败 不影响服务启动

func init() {
	if !property.GetBool("skywalking.enabled") {
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
	gr, err := reporter.NewGRPCReporter(serverAddr,
		reporter.WithLog(logger.Logger),
		reporter.WithMaxSendQueueSize(maxSendQueueSize))
	if err != nil {
		logger.Logger.Error(err)
		return
	}
	samplerRate := 0.6
	if property.GetFloat64("skywalking.samplerRate") > 0 {
		samplerRate = property.GetFloat64("skywalking.samplerRate")
	}
	tracer, err := go2sky.NewTracer(app.ApplicationName,
		go2sky.WithReporter(gr),
		go2sky.WithSampler(samplerRate),
	)
	//动态调整采样率
	property_loader.RegisterKeyChangeWatcher("skywalking.samplerRate", func() {
		rate := property.GetFloat64("skywalking.samplerRate")
		logger.Logger.Info("skywalking.samplerRate changed:", rate)
		go2sky.NewDynamicSampler(rate, tracer)
	})
	if err != nil {
		gr.Close()
		logger.Logger.Error(err)
		return
	}
	Tracer = tracer
	logger.Logger.Info("skywalking start tracer:", serverAddr)
	quit.RegisterQuitFunc(func() {
		gr.Close()
	})
}
