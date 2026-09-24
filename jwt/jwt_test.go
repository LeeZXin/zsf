package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndValidate(t *testing.T) {
	tok, err := GenerateToken("alice", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Account != "alice" {
		t.Fatalf("account=%s", claims.Account)
	}
}

func TestValidateExpired(t *testing.T) {
	tok, err := GenerateToken("bob", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateToken(tok); err == nil {
		t.Fatal("过期 token 应校验失败")
	}
}

func TestValidateTampered(t *testing.T) {
	if _, err := ValidateToken("not-a-jwt"); err == nil {
		t.Fatal("非法 token 应校验失败")
	}
}
