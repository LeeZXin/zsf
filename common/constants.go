package common

import (
	"flag"
	"github.com/LeeZXin/zsf/logger"
)

// 全局变量 如环境 版本号 ip等
var (
	Env     string
	LocalIP string
	Version string
)

const (
	DefaultVersion = "default"
	HttpScheme     = "http"
	GrpcScheme     = "grpc"
	VersionPrefix  = "version="
)

var (
	env = flag.String("env", "", "app env")
	ver = flag.String("ver", "", "app version")
)

func init() {
	if !flag.Parsed() {
		flag.Parse()
	}
	if ver == nil || *ver == "" {
		Version = DefaultVersion
	} else {
		Version = *ver
	}
	logger.Logger.Info("project version is ", Version)
	finalEnv := *env
	if finalEnv == "" {
		logger.Logger.Panic("project env is nil")
	}
	logger.Logger.Info("project env is ", finalEnv)
	Env = finalEnv
	//获取本地ip
	LocalIP = getLocalIp()
	if LocalIP == "" {
		logger.Logger.Panic("can not get local ipv4")
	} else {
		logger.Logger.Info("get local ipv4: ", LocalIP)
	}

}

func getLocalIp() string {
	ips := AllIPV4()
	if ips == nil || len(ips) == 0 {
		return ""
	}
	return ips[0]
}
