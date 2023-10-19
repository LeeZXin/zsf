package common

import (
	"github.com/LeeZXin/zsf-utils/idutil"
	"github.com/LeeZXin/zsf-utils/iputil"
	"github.com/LeeZXin/zsf/property/static"
)

var (
	applicationName string
	region          string
	zone            string

	localIP string

	instanceId = idutil.RandomUuid()
)

const (
	DefaultVersion = "default"
	HttpProtocol   = "http"
	GrpcProtocol   = "grpc"
	VersionPrefix  = "version="
)

func init() {
	//获取applicationName
	applicationName = static.GetString("application.name")
	if applicationName == "" {
		applicationName = idutil.RandomUuid()
	}
	//region
	region = static.GetString("application.region")
	if region == "" {
		region = "#"
	}
	//zone
	zone = static.GetString("application.zone")
	if zone == "" {
		zone = "#"
	}
	//获取本地ip
	localIP = iputil.GetIPV4()
	if localIP == "" {
		panic("can not get local ipv4")
	}
}

func GetApplicationName() string {
	return applicationName
}

func GetRegion() string {
	return region
}

func GetZone() string {
	return zone
}

func GetLocalIP() string {
	return localIP
}

func GetInstanceId() string {
	return instanceId
}
