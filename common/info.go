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

	httpServerPort int
)

const (
	DefaultVersion = "default"
	HttpProtocol   = "http"

	DefaultHttpServerPort = 15003

	ResourcesDir   = "resources"
	ServicePrefix  = "/services/"
	PropertyPrefix = "/property/"
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
	localIP = static.GetString("application.ip")
	if localIP == "" {
		localIP = iputil.GetIPV4()
	}
	httpServerPort = static.GetInt("http.port")
	if httpServerPort <= 0 {
		httpServerPort = DefaultHttpServerPort
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

func HttpServerPort() int {
	return httpServerPort
}
