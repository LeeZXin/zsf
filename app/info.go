package app

import (
	"zsf/logger"
	"zsf/property"
)

var (
	ApplicationName string
	Region          string
	Zone            string
)

func init() {
	//获取applicationName
	applicationName := property.GetString("application.name")
	if applicationName == "" {
		logger.Logger.Panic("nil applicationName")
	}
	ApplicationName = applicationName
	//region
	region := property.GetString("application.region")
	if region == "" {
		region = "#"
	}
	Region = region
	//zone
	zone := property.GetString("application.zone")
	if zone == "" {
		zone = "#"
	}
	Zone = zone
}
