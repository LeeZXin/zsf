package strutil

import (
	"math/rand"
	"strings"
)

// RandomStr 指定长度随机字符串
func RandomStr(length int) string {
	if length <= 0 {
		return ""
	}
	sb := strings.Builder{}
	for i := 0; i < length; i++ {
		sb.WriteString(c62[rand.Intn(62)])
	}
	return sb.String()
}
