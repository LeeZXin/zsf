// Package validateutil 提供字符串参数校验工具：长度校验（非空/可空）、
// 格式校验（IPv4/邮箱/URL/UUID）与组合校验规则，校验失败时返回 false。
package validateutil

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// NotEmptyStr 校验字符串去除首尾空白后非空且长度不超过 length（按字节计）。
// 注意：会原地修改 *str（去掉首尾空白）；length 是字节上限而非字符数。
func NotEmptyStr(str *string, length int) bool {
	*str = strings.TrimSpace(*str)
	return len(*str) > 0 && len(*str) <= length
}

// NotEmptyStrLen 返回固定长度上限的非空校验函数，供 Rules 组合使用。
func NotEmptyStrLen(length int) func(*string) bool {
	return func(str *string) bool {
		return NotEmptyStr(str, length)
	}
}

// NotEmptyStrLenLe32 等价于 NotEmptyStr(str, 32)：非空且长度不超过 32。
func NotEmptyStrLenLe32(str *string) bool {
	return NotEmptyStr(str, 32)
}

// NotEmptyStrLenLe64 等价于 NotEmptyStr(str, 64)：非空且长度不超过 64。
func NotEmptyStrLenLe64(str *string) bool {
	return NotEmptyStr(str, 64)
}

// NotEmptyStrLenLe128 等价于 NotEmptyStr(str, 128)：非空且长度不超过 128。
func NotEmptyStrLenLe128(str *string) bool {
	return NotEmptyStr(str, 128)
}

// NotEmptyStrLenLe512 等价于 NotEmptyStr(str, 512)：非空且长度不超过 512。
func NotEmptyStrLenLe512(str *string) bool {
	return NotEmptyStr(str, 512)
}

// NotEmptyStrLenLe1024 等价于 NotEmptyStr(str, 1024)：非空且长度不超过 1024。
func NotEmptyStrLenLe1024(str *string) bool {
	return NotEmptyStr(str, 1024)
}

// NotEmptyStrLenLe2048 等价于 NotEmptyStr(str, 2048)：非空且长度不超过 2048。
func NotEmptyStrLenLe2048(str *string) bool {
	return NotEmptyStr(str, 2048)
}

// NotEmptyStrLenLe65535 等价于 NotEmptyStr(str, 65535)：非空且长度不超过 65535。
func NotEmptyStrLenLe65535(str *string) bool {
	return NotEmptyStr(str, 65535)
}

// AllowEmptyStr 校验字符串去除首尾空白后长度不超过 length（允许为空），按字节计。
// 注意：会原地修改 *str（去掉首尾空白）。
func AllowEmptyStr(str *string, length int) bool {
	*str = strings.TrimSpace(*str)
	return len(*str) <= length
}

// AllowEmptyStrLen 返回固定长度上限的可空校验函数，供 Rules 组合使用。
func AllowEmptyStrLen(length int) func(*string) bool {
	return func(str *string) bool {
		return AllowEmptyStr(str, length)
	}
}

// AllowEmptyStrLenLe32 等价于 AllowEmptyStr(str, 32)：长度不超过 32，允许为空。
func AllowEmptyStrLenLe32(str *string) bool {
	return AllowEmptyStr(str, 32)
}

// AllowEmptyStrLenLe64 等价于 AllowEmptyStr(str, 64)：长度不超过 64，允许为空。
func AllowEmptyStrLenLe64(str *string) bool {
	return AllowEmptyStr(str, 64)
}

// AllowEmptyStrLenLe128 等价于 AllowEmptyStr(str, 128)：长度不超过 128，允许为空。
func AllowEmptyStrLenLe128(str *string) bool {
	return AllowEmptyStr(str, 128)
}

// AllowEmptyStrLenLe512 等价于 AllowEmptyStr(str, 512)：长度不超过 512，允许为空。
func AllowEmptyStrLenLe512(str *string) bool {
	return AllowEmptyStr(str, 512)
}

// AllowEmptyStrLenLe1024 等价于 AllowEmptyStr(str, 1024)：长度不超过 1024，允许为空。
func AllowEmptyStrLenLe1024(str *string) bool {
	return AllowEmptyStr(str, 1024)
}

// AllowEmptyStrLenLe2048 等价于 AllowEmptyStr(str, 2048)：长度不超过 2048，允许为空。
func AllowEmptyStrLenLe2048(str *string) bool {
	return AllowEmptyStr(str, 2048)
}

// AllowEmptyStrLenLe65535 等价于 AllowEmptyStr(str, 65535)：长度不超过 65535，允许为空。
func AllowEmptyStrLenLe65535(str *string) bool {
	return AllowEmptyStr(str, 65535)
}

// Rule 是一条校验规则：Field 为待校验字段的指针，Validate 为该字段的校验函数。
type Rule struct {
	Field    *string
	Validate func(*string) bool
}

// Rules 是规则列表，按顺序依次校验。
type Rules []Rule

// Do 依次执行全部规则，任一规则校验失败立即返回 false；全部通过返回 true。
func (rules Rules) Do() bool {
	for _, r := range rules {
		if !r.Validate(r.Field) {
			return false
		}
	}
	return true
}

var (
	ipRegexp    = regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])$`)
	emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

// IsIpV4 校验字符串是否为合法的 IPv4 地址（点分十进制格式）。
// 注意：不修改参数；空字符串返回 false。
func IsIpV4(ip *string) bool {
	return ipRegexp.MatchString(*ip)
}

// IsEmail 校验字符串是否为合法邮箱，且长度不超过 128 字节。
func IsEmail(email *string) bool {
	return emailRegexp.MatchString(*email) && len(*email) <= 128
}

// IsUrl 校验字符串能否被 url.Parse 解析。
// 注意：校验较宽松，仅验证语法合法性，不保证 URL 可访问。
func IsUrl(u *string) bool {
	_, err := url.Parse(*u)
	return err == nil
}

// IsUUID 校验并规范为带连字符的 UUID（Claude --session-id / --resume 只认这个）。
// 注意：会原地 TrimSpace，通过后写成 uuid.UUID.String()。
func IsUUID(s *string) bool {
	*s = strings.TrimSpace(*s)
	id, err := uuid.Parse(*s)
	if err != nil {
		return false
	}
	*s = id.String()
	return true
}

// SlicesContains 返回一个校验函数：字符串必须在白名单列表中。
// 注意：不做空白处理，匹配区分大小写。
func SlicesContains(list ...string) func(*string) bool {
	return func(str *string) bool {
		return slices.Contains(list, *str)
	}
}
