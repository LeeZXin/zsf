package main

import (
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/starter"
)

func main() {
	logger.Logger.Info("hello world")
	starter.Run()
}
