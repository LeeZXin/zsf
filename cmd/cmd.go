package cmd

import (
	"flag"
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
	env = *envCmd
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
