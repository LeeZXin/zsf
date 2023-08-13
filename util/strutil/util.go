package strutil

import (
	"fmt"
	"math/rand"
	"strings"
)

var (
	c62 = []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
	}
)

// To62Str 数字转化为62进制字符串
func To62Str(i uint64) string {
	sb := strings.Builder{}
	for i > 0 {
		sb.WriteString(c62[i%62])
		i /= 62
	}
	return sb.String()
}

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

func Concat(data []any, delimiter string) string {
	if data == nil || len(data) == 0 {
		return ""
	}
	ret := strings.Builder{}
	for i, d := range data {
		ret.WriteString(fmt.Sprintf("%v", d))
		if i < len(data)-1 {
			ret.WriteString(delimiter)
		}
	}
	return ret.String()
}
