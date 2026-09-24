package ginutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCacheHeaderRFC1123(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	CacheHeader(c, time.Hour)
	if _, err := http.ParseTime(w.Header().Get("Date")); err != nil {
		t.Fatalf("Date 应为 HTTP 时间格式: %v, val=%q", err, w.Header().Get("Date"))
	}
	if _, err := http.ParseTime(w.Header().Get("Expires")); err != nil {
		t.Fatalf("Expires 应为 HTTP 时间格式: %v, val=%q", err, w.Header().Get("Expires"))
	}
	if w.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("Cache-Control=%q", w.Header().Get("Cache-Control"))
	}
}
