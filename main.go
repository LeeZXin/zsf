package main

import (
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	//dynamic.Init()
	//zsf.Run(
	//	zsf.WithDiscovery(discovery.NewEtcdDiscovery()),
	//	zsf.WithLifeCycles(
	//		httpserver.NewServer(
	//			httpserver.WithRegistryAction(
	//				registry.NewDefaultHttpAction(
	//					registry.NewDefaultEtcdRegistry(),
	//				),
	//			),
	//		),
	//		actuator.NewServer(),
	//		prom.NewServer(),
	//		pprof.NewServer(),
	//	),
	//)
	go func() {
		for k := 0; k < 10000; k++ {
			logger.Logger.Info(idutil.RandomUuid())
		}
	}()
	go func() {
		for k := 0; k < 10000; k++ {
			logger.Logger.Error(idutil.RandomUuid())
		}
	}()
	go func() {
		for k := 0; k < 10000; k++ {
			logger.Logger.Warn(idutil.RandomUuid())
		}
	}()

	zsf.Run()
}
