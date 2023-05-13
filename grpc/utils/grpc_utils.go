package grpcutils

import (
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

// grpc相关工具类

var (
	jsm = jsonpb.Marshaler{EmitDefaults: true, Indent: ""}
)

// PrintJsonMessage 将pb转成jsonString
func PrintJsonMessage(msg proto.Message) (string, error) {
	return jsm.MarshalToString(msg)
}
