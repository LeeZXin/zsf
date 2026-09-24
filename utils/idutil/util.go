package idutil

import (
	"strings"

	"github.com/google/uuid"
)

// RandomUUID 生成随机UUID（无连字符）
// 返回: 32位十六进制格式的随机UUID字符串
func RandomUUID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// RawUUID 生成带连字符的 RFC 4122 UUID，给 Claude --session-id 这类要标准格式的地方。
func RawUUID() string {
	return uuid.New().String()
}
