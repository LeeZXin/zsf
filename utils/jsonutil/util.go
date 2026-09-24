// Package jsonutil 提供基于 sonic 的 JSON 序列化工具。
// 注意：序列化错误被忽略，序列化失败时返回空字符串或 nil，调用方无法感知失败。
package jsonutil

import "github.com/bytedance/sonic"

// MarshalStringIgnoreErr 将任意值序列化为 JSON 字符串，失败时返回空字符串（错误被吞掉）。
// 适用于日志输出、缓存值拼接等对序列化失败不敏感的场景。
func MarshalStringIgnoreErr(v any) string {
	m, _ := sonic.MarshalString(v)
	return m
}

// MarshalIgnoreErr 将任意值序列化为 JSON 字节切片，失败时返回 nil（错误被吞掉）。
func MarshalIgnoreErr(v any) []byte {
	m, _ := sonic.Marshal(v)
	return m
}
