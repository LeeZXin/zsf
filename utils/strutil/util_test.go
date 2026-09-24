package strutil

import (
	"strings"
	"testing"
)

func TestRandomStr4CryptoLengthAndAlphabet(t *testing.T) {
	if got := RandomStr4Crypto(0); got != "" {
		t.Fatalf("length<=0 应为空, got %q", got)
	}
	s := RandomStr4Crypto(32)
	if len(s) != 32 {
		t.Fatalf("len=%d", len(s))
	}
	allowed := strings.Join(c62, "")
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(allowed, rune(s[i])) {
			t.Fatalf("非法字符 %q", s[i])
		}
	}
	if RandomStr4Crypto(32) == s {
		t.Fatal("连续两次生成不应相同")
	}
}
