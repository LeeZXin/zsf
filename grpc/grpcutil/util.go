// Package grpcutil 提供 gRPC 消息（proto.Message）与 Go 对象/字符串之间的
// JSON 互转工具，以及 gRPC 方法名的路径格式化。
//
// 转换链路统一走 protobuf 官方 protojson（保证 proto 字段命名与类型语义），
// 对象侧使用 sonic 高性能 JSON 库。protojson 与 sonic 的 JSON 形态存在差异
// （如 int64 编码为字符串、字段名大小写），互转仅适用于日志/调试/透传场景，
// 不应作为跨服务的数据契约。
package grpcutil

import (
	"strings"

	"github.com/bytedance/sonic"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	marshaler = protojson.MarshalOptions{
		UseProtoNames:   false,
		EmitUnpopulated: false,
	}
	marshalerEmitAll = protojson.MarshalOptions{
		UseProtoNames:   false,
		EmitUnpopulated: true,
	}
	unmarshaler = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

// MessageToObject 将 proto.Message 经 JSON 序列化后解析到对象 obj 中。
// Marshaler 未开启 EmitUnpopulated，proto 中的零值字段会被省略（obj 对应字段
// 保持零值）；序列化失败或字段类型不匹配时返回 error。
func MessageToObject(m proto.Message, obj any) error {
	b, err := marshaler.Marshal(m)
	if err != nil {
		return err
	}
	return sonic.Unmarshal(b, obj)
}

// ObjectToMessage 将对象 obj 经 JSON 序列化后解析到 proto.Message 中。
// Unmarshaler 开启了 DiscardUnknown：obj 中与 proto 定义不符的字段会被
// 静默忽略——拼错字段名不会报错，排查转换异常时先核对字段名。
func ObjectToMessage(obj any, msg proto.Message) error {
	m, err := sonic.Marshal(obj)
	if err != nil {
		return err
	}
	return unmarshaler.Unmarshal(m, msg)
}

// PackMethodName 将形如 "pkg.Service.Method" 的方法全名格式化为 gRPC 调用
// 路径 "/pkg.Service/Method"；name 中不含 "."（已按 gRPC 路径格式传入）时
// 原样返回。
func PackMethodName(name string) string {
	index := strings.LastIndex(name, ".")
	if index == -1 {
		return name
	}
	return "/" + name[:index] + "/" + name[index+1:]
}

// MessageToString 将 proto.Message 序列化为 JSON 字符串
// （EmitUnpopulated=true，零值字段也会输出），主要用于日志打印。
func MessageToString(msg proto.Message) (string, error) {
	b, err := marshalerEmitAll.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
