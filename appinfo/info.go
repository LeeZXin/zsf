package appinfo

import (
	"flag"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/property"
	"net"
)

var (
	ApplicationName string
	Region          string
	Zone            string

	Env     string
	LocalIP string
	Version string

	env = flag.String("env", "", "app env")
	ver = flag.String("ver", "", "app version")
)

const (
	DefaultVersion = "default"
	HttpScheme     = "http"
	GrpcScheme     = "grpc"
	VersionPrefix  = "version="
)

func init() {
	//获取applicationName
	ApplicationName = property.GetString("application.name")
	if ApplicationName == "" {
		logger.Logger.Panic("nil applicationName")
	}

	//region
	Region = property.GetString("application.region")
	if Region == "" {
		Region = "#"
	}

	//zone
	Zone = property.GetString("application.zone")
	if Zone == "" {
		Zone = "#"
	}

	//服务版本号
	if !flag.Parsed() {
		flag.Parse()
	}
	if ver == nil || *ver == "" {
		Version = DefaultVersion
	} else {
		Version = *ver
	}
	logger.Logger.Info("project version is ", Version)
	Env = *env
	if Env == "" {
		logger.Logger.Panic("project env is nil")
	}
	logger.Logger.Info("project env is ", Env)

	//获取本地ip
	LocalIP = getLocalIp()
	if LocalIP == "" {
		logger.Logger.Panic("can not get local ipv4")
	} else {
		logger.Logger.Info("get local ipv4: ", LocalIP)
	}
}

func getLocalIp() string {
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
