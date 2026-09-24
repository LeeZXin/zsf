// Package hashutil 提供字符串哈希/摘要工具：MD5、SHA-256 与加盐密码哈希。
package hashutil

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
)

// Md5 返回字符串的 MD5 摘要（32 位十六进制小写）。
// 注意：MD5 存在碰撞风险，仅适用于一致性校验等场景，勿用于安全敏感用途。
func Md5(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

// Sha256 返回字符串的 SHA-256 摘要（64 位十六进制小写）。
func Sha256(str string) string {
	h := sha256.New()
	h.Write([]byte(str))
	return hex.EncodeToString(h.Sum(nil))
}

// HashPassword 计算 salt 与 pwd 拼接后的 SHA-256 摘要，作为密码哈希。
// 注意：这是简单加盐哈希（非 bcrypt/scrypt 等慢哈希），且不存储随机盐，
// 仅适用于内部低安全要求的场景；对外服务建议改用专门的密码哈希算法。
func HashPassword(salt, pwd string) string {
	return Sha256(salt + pwd)
}
