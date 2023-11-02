package main

import (
	_ "github.com/LeeZXin/zsf/grpc/grpcserver"
	_ "github.com/LeeZXin/zsf/http/httpserver"
	"github.com/LeeZXin/zsf/property/dynamic"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	dynamic.GetString("string")
	zsf.Run()
}
