package aesutil

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := Generate16AesKey()
	nonce := GenerateIV()
	if len(key) != 16 || len(nonce) != gcmNonceSize {
		t.Fatalf("key/nonce 长度错误: %d %d", len(key), len(nonce))
	}
	plain := []byte("hello zsf aes")
	enc, err := AesEncrypt(plain, nonce, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := AesDecrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q", got)
	}
}

func TestEncryptRejectsBadNonceSize(t *testing.T) {
	key := Generate16AesKey()
	if _, err := AesEncrypt([]byte("x"), Generate16AesKey(), key); err == nil {
		t.Fatal("16 字节 nonce 应拒绝")
	}
}

func TestDecryptRejectsTamper(t *testing.T) {
	key := Generate16AesKey()
	enc, err := AesEncrypt([]byte("hello"), GenerateIV(), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	if _, err := AesDecrypt(hex.EncodeToString(raw), key); err == nil {
		t.Fatal("篡改密文应认证失败")
	}
}

func TestDecryptEmptyPlaintext(t *testing.T) {
	key := Generate16AesKey()
	enc, err := AesEncrypt(nil, GenerateIV(), key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := AesDecrypt(enc, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}
