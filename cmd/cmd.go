package cmd

import (
	"flag"
	"os"
)

var (
	env     string
	version string
)

const (
	DefaultEnv     = "sit"
	DefaultVersion = "default"
)

func init() {
	//服务版本号
	if !flag.Parsed() {
		flag.Parse()
	}
	version = os.Getenv("ZSF_VERSION")
	if version == "" {
		version = DefaultVersion
	}
	env = os.Getenv("ZSF_ENV")
	if env == "" {
		env = DefaultEnv
	}
}

func GetEnv() string {
	return env
}

func GetVersion() string {
	return version
}
