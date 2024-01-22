package env

import (
	"os"
)

var (
	env      string
	version  string
	nodeFlag string
)

const (
	DefaultEnv     = "sit"
	DefaultVersion = "default"
)

func init() {
	//服务版本号
	version = os.Getenv("ZSF_VERSION")
	if version == "" {
		version = DefaultVersion
	}
	env = os.Getenv("ZSF_ENV")
	if env == "" {
		env = DefaultEnv
	}
	nodeFlag = os.Getenv("ZSF_NODE_FLAG")
}

func GetEnv() string {
	return env
}

func GetVersion() string {
	return version
}

func GetNodeFlag() string {
	return nodeFlag
}
