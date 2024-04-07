package main

import (
	"fmt"
	"github.com/LeeZXin/zsf/actuator"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	fmt.Println(static.GetStringSlice("redis.read.cmd"))
	zsf.Run(
		zsf.WithDiscovery(discovery.NewStaticDiscovery()),
		zsf.WithLifeCycles(
			httpserver.NewServer(
				httpserver.WithRegistryAction(
					registry.NewDefaultHttpAction(
						registry.NewDefaultEtcdRegistry(),
					),
				),
			),
			actuator.NewServer(),
			prom.NewServer(),
			pprof.NewServer(),
		),
	)
}
