package sentinelutil

import (
	"github.com/LeeZXin/zsf/instance"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/start"

	"github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/config"
)

func init() {
	start.AddInit(func() {
		conf := config.NewDefaultConfig()
		conf.Sentinel.App.Name = instance.ApplicationName
		conf.Sentinel.Log.Logger = new(slogger)
		conf.Sentinel.Log.Metric.FlushIntervalSec = 0
		conf.Sentinel.Stat.System.CollectIntervalMs = 0
		err := api.InitWithConfig(conf)
		if err != nil {
			logger.Logger.Fatal().Msgf("Sentinel init failed: %v", err)
		}
	}, -4)
}
