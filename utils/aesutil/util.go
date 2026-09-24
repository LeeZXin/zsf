// Package aesutil 提供 AES-GCM 加解密工具（nonce 前置拼接在密文头部，AEAD 自带完整性校验），
// 密文以 hex 编码字符串对外传输；同时提供随机 AES 密钥与 nonce 的生成函数。
//
// 密文布局：nonce(12 字节) || ciphertext || tag(16 字节)。与旧版 AES-CBC 密文不兼容。
package aesutil

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/LeeZXin/zsf/utils/strutil"
)

// gcmNonceSize AES-GCM 推荐 nonce 长度（96 bit）。同一密钥下 nonce 不可复用。
const gcmNonceSize = 12

func aesEncrypt(plaintext, nonce, key []byte) ([]byte, error) {
	if len(nonce) != gcmNonceSize {
		return nil, fmt.Errorf("nonce length is wrong: %d != %d", len(nonce), gcmNonceSize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func aesDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	minLen := gcmNonceSize + gcm.Overhead()
	if len(ciphertext) < minLen {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcmNonceSize]
	sealed := ciphertext[gcmNonceSize:]
	return gcm.Open(nil, nonce, sealed, nil)
}

// AesEncrypt 使用 AES-GCM 加密 content，nonce 前置拼接在密文头部，结果以 hex 编码字符串返回。
// 要求：nonce 必须为 12 字节；aesKey 长度必须为 16/24/32 字节（对应 128/192/256 位），否则返回 error。
// 同一 aesKey 下每次加密须使用新 nonce（见 GenerateIV）。
func AesEncrypt(content []byte, nonce, aesKey string) (string, error) {
	encrypt, err := aesEncrypt(content, []byte(nonce), []byte(aesKey))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(encrypt), nil
}

// AesDecrypt 解密 AesEncrypt 产生的 hex 密文（nonce 从密文头部自动取出）。
// hex 解码失败、密文过短或 GCM 认证失败（被篡改/密钥错误）时返回 error。
// aesKey 须与加密时保持一致。
func AesDecrypt(content, aesKey string) ([]byte, error) {
	encrypted, err := hex.DecodeString(content)
	if err != nil {
		return nil, err
	}
	return aesDecrypt(encrypted, []byte(aesKey))
}

func generateAesKey(keyLen int) (string, error) {
	key := strutil.RandomStr4Crypto(keyLen)
	if key == "" {
		return "", errors.New("generate aes key failed")
	}
	_, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	return key, nil
}

// Generate16AesKey 生成 16 字节随机 AES 密钥（128 位），返回可打印字符串。
// 熵来自 crypto/rand（strutil.RandomStr4Crypto）。
func Generate16AesKey() string {
	key, _ := generateAesKey(16)
	return key
}

// Generate24AesKey 生成 24 字节随机 AES 密钥（192 位），返回可打印字符串。
func Generate24AesKey() string {
	key, _ := generateAesKey(24)
	return key
}

// Generate32AesKey 生成 32 字节随机 AES 密钥（256 位），返回可打印字符串。
func Generate32AesKey() string {
	key, _ := generateAesKey(32)
	return key
}

// GenerateIV 生成 12 字节随机 nonce 字符串，用于 AES-GCM 加密（与 AesEncrypt 的 nonce 参数配套）。
// 同一密钥下不可复用；每次加密调用一次。
func GenerateIV() string {
	return strutil.RandomStr4Crypto(gcmNonceSize)
}
