package cmd

import (
	"flag"
	"os"
)

var (
	env     string
	version string

	envCmd = flag.String("env", "", "app env")
	verCmd = flag.String("ver", "", "app version")
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
	if verCmd == nil || *verCmd == "" {
		version = os.Getenv("ZSF_VERSION")
		if version == "" {
			version = DefaultVersion
		}
	} else {
		version = *verCmd
	}
	if envCmd == nil || *envCmd == "" {
		env = os.Getenv("ZSF_ENV")
		if env == "" {
			env = DefaultEnv
		}
	} else {
		env = *envCmd
	}
}

func GetEnv() string {
	return env
}

func GetVersion() string {
	return version
}
