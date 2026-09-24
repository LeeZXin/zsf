/*
Package strutil 提供字符串生成与处理工具：
  - To62Str：数字转 62 进制短串（短链接、邀请码）
  - RandomStr / RandomNumStr：随机字符串 / 纯数字随机串
  - RandomStr4Crypto：密码学随机源的字母数字串（密钥、IV 等）
  - Concat：任意类型切片拼接
  - MaxBytes：按字节截断（注意可能截断出非法 UTF-8 序列）
  - Replacer：链式字符串替换器
*/
package strutil

import (
	"crypto/rand"
	"fmt"
	mathrand "math/rand/v2"
	"strings"
)

var (
	/*
		c62 是 62 进制字符表：[0-9a-zA-Z]，用于 To62Str 编码和 RandomStr / RandomStr4Crypto 生成。
		顺序是数字 → 小写字母 → 大写字母。
	*/
	c62 = []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
	}
	/*
		n10 是十进制数字字符表 [0-9]，用于 RandomNumStr 生成纯数字随机串。
	*/
	n10 = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
)

/*
To62Str 将 uint64 数字转换为 62 进制字符串。

62 进制字符表为 [0-9a-zA-Z]，共计 62 个字符。
适用场景：将数字 ID 编码为更短的字符串，用于短链接、邀请码等。

注意：结果为倒序的（类似进制转换的余数倒排），高位在左侧。
输入 0 返回空字符串。
*/
func To62Str(i uint64) string {
	sb := strings.Builder{}
	for i > 0 {
		sb.WriteString(c62[i%62])
		i /= 62
	}
	return sb.String()
}

/*
RandomStr 生成指定长度的随机字符串（从 62 进制字符表中均匀随机选取）。

length <= 0 时返回空字符串。
使用 math/rand/v2（Go 1.22+ 的随机数 API），线程安全。
仅用于短链、邀请码等非密钥场景；密钥/IV 请用 RandomStr4Crypto。
*/
func RandomStr(length int) string {
	if length <= 0 {
		return ""
	}
	sb := strings.Builder{}
	for range length {
		sb.WriteString(c62[mathrand.IntN(62)])
	}
	return sb.String()
}

/*
RandomStr4Crypto 生成指定长度的密码学安全随机字符串（字符表与 RandomStr 相同）。

熵来自 crypto/rand；对字节做拒绝采样后再映射到 62 字符表，避免取模偏差。
length <= 0 时返回空字符串。读取 CSPRNG 失败时 panic（主机熵源异常，无法继续）。
适用场景：JWT 兜底密钥、AES 密钥/IV 等需要不可预测性的可打印 secret。
*/
func RandomStr4Crypto(length int) string {
	if length <= 0 {
		return ""
	}
	const alphabetLen = 62
	// 256 % 62 = 8，丢弃 [248,255]，其余字节 % 62 均匀
	const maxUnbiased = 256 - (256 % alphabetLen)
	out := make([]byte, 0, length)
	buf := make([]byte, length)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, b := range buf {
			if int(b) >= maxUnbiased {
				continue
			}
			out = append(out, c62[int(b)%alphabetLen][0])
			if len(out) == length {
				break
			}
		}
	}
	return string(out)
}

/*
RandomNumStr 生成指定长度的随机数字字符串（纯数字 [0-9]）。

适用场景：生成短信验证码、随机数字后缀（如 Snowflake ID 的 4 位尾号）。
length <= 0 时返回空字符串。
*/
func RandomNumStr(length int) string {
	if length <= 0 {
		return ""
	}
	sb := strings.Builder{}
	for range length {
		sb.WriteString(n10[mathrand.IntN(10)])
	}
	return sb.String()
}

/*
Concat 将任意类型的切片按分隔符拼接为字符串。

元素通过 fmt.Sprintf("%v", d) 转换为字符串，因此支持任意类型。
delimiter 放在相邻两个元素之间，尾部没有多余的分隔符。

示例:

	Concat([]any{1, "hello", 3.14}, ", ") → "1, hello, 3.14"
	Concat([]any{}, ",") → ""
*/
func Concat(data []any, delimiter string) string {
	if len(data) == 0 {
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

/*
MaxBytes 按字节截断字符串，返回不超过 length 字节的前缀。

length <= 0 时返回空字符串。
注意：这是按字节截断，而非按 rune（字符）。对于多字节 UTF-8 字符，
可能在字符中间截断产生非法 UTF-8 序列。如需安全截断，调用方应额外处理。

适用场景：数据库字段长度截断、日志输出限制等。
*/
func MaxBytes(str string, length int) string {
	if length <= 0 {
		return ""
	}
	if len(str) > length {
		return str[0:length]
	}
	return str
}
