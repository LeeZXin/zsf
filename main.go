package main

import (
	"github.com/LeeZXin/zsf/actuator"
	"github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/pprof"
	"github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/services/registry"
	"github.com/LeeZXin/zsf/starter"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	starter.Run(
		zsf.WithLifeCycles(
			httpserver.NewServer(
				httpserver.WithRegistryAction(
					registry.NewDefaultHttpAction(
						registry.NewEtcdRegistry(),
					),
				),
			),
			actuator.NewServer(),
			prom.NewServer(),
			pprof.NewServer(),
		),
	)
}
