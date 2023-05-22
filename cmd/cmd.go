package cmd

import (
	"flag"
	"github.com/LeeZXin/zsf/logger"
)

var (
	env     string
	version string

	envCmd = flag.String("env", "", "app env")
	verCmd = flag.String("ver", "", "app version")
)

const (
	DefaultVersion = "default"
	DefaultEnv     = "sit"
)

func init() {
	//服务版本号
	if !flag.Parsed() {
		flag.Parse()
	}
	if verCmd == nil || *verCmd == "" {
		version = DefaultVersion
	} else {
		version = *verCmd
	}
	logger.Logger.Info("project version is ", version)
	env = *envCmd
	if env == "" {
		logger.Logger.Info("project env is nil set to default: ", DefaultEnv)
		env = DefaultEnv
	}
	logger.Logger.Info("project env is ", env)
}

func GetEnv() string {
	return env
}

func GetVersion() string {
	return version
}
