package main

import (
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/property/dynamic"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	dynamic.Init()
	zsf.Run(
		zsf.WithDiscovery(discovery.NewEtcdDiscovery()),
		zsf.WithLifeCycles(
			httpserver.NewServer(
				httpserver.WithRegistry(
					registry.NewDefaultEtcdRegistry(),
				),
				httpserver.WithEnableActuator(true),
				httpserver.WithEnablePromApi(true),
				httpserver.WithEnablePprof(true),
			),
		),
	)
}
