package main

import (
	"github.com/LeeZXin/zsf/actuator"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	//dynamic.Init()
	zsf.Run(
		zsf.WithDiscovery(discovery.NewEtcdDiscovery()),
		zsf.WithLifeCycles(
			httpserver.NewServer(
				httpserver.WithRegistryAction(
					registry.NewDefaultHttpAction(
						registry.NewDefaultEtcdRegistry(),
					),
				),
			),
			actuator.NewServer(),
			//prom.NewServer(),
			//pprof.NewServer(),
		),
	)
}
