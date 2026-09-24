// Package jwt 提供基于 HS256 签名的 JWT 签发与校验，用于用户身份凭证。
//
// 密钥来自静态配置 jwt.secret。未配置时：
//   - 生产环境（SF_ENV=prd）直接 Fatal，禁止用随机密钥上线
//   - 其它环境进程启动用 crypto/rand 生成 16 位密钥——该兜底仅用于本地开发，
//     进程重启会导致已签发 token 全部失效
//
// Claims 内嵌 jwt.RegisteredClaims（含 exp 过期时间），ValidateToken 会校验
// 签名算法（固定 HS256，防算法混淆攻击）与过期时间。
package jwt

import (
	"fmt"
	"log"
	"time"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/utils/strutil"

	"github.com/golang-jwt/jwt/v5"
)

var (
	secretKey string
)

func init() {
	start.AddInit(func() {
		secretKey = static.GetString("jwt.secret")
		if secretKey == "" {
			if env.Env == env.PrdEnv {
				log.Fatalln("jwt.secret is required when SF_ENV=prd")
			}
			secretKey = strutil.RandomStr4Crypto(16)
		}
	}, -6)
}

// Claims JWT 载荷：Account 为登录用户账号；RegisteredClaims 提供标准
// 过期时间等字段（签发时通过 ExpiresAt 设置，校验时自动检查）。
type Claims struct {
	Account string `json:"account"`
	jwt.RegisteredClaims
}

// GenerateToken 为 account 签发 HS256 签名的 token，expired 为过期时间点；
// 返回 base64url 编码的 JWT 字符串。token 仅包含账号与标准过期字段，
// 不携带其他业务信息。
func GenerateToken(account string, expired time.Time) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		Account:   account,
		ExpiresAt: jwt.NewNumericDate(expired),
	}).SignedString([]byte(secretKey))
}

// ValidateToken 校验 token 的签名算法（必须为 HS256，拒绝算法混淆）与
// 过期时间，校验通过返回 Claims；签名无效、被篡改或已过期均返回 error，
// 调用方应据此拒绝请求。
func ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, new(Claims), func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
