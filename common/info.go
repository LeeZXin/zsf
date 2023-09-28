package common

import (
	"github.com/LeeZXin/zsf/property"
	"github.com/LeeZXin/zsf/util/idutil"
	"github.com/google/uuid"
	"net"
	"strings"
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
	applicationName = property.GetString("application.name")
	if applicationName == "" {
		applicationName = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	//region
	region = property.GetString("application.region")
	if region == "" {
		region = "#"
	}
	//zone
	zone = property.GetString("application.zone")
	if zone == "" {
		zone = "#"
	}
	//获取本地ip
	localIP = getIp()
	if localIP == "" {
		panic("can not get local ipv4")
	}
}

func getIp() string {
	ips := allIPV4()
	if ips == nil || len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func allIPV4() (ipv4s []string) {
	adders, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	for _, addr := range adders {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ipv4 := ipNet.IP.String()
				if ipv4 == "127.0.0.1" || ipv4 == "localhost" {
					continue
				}
				ipv4s = append(ipv4s, ipv4)
			}
		}
	}
	return
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

func GetLocalIp() string {
	return localIP
}

func GetInstanceId() string {
	return instanceId
}
