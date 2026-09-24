package autohttps

import (
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"golang.org/x/crypto/acme"
)

func TestNeedsRenewal(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	cases := []struct {
		name      string
		notBefore time.Time
		notAfter  time.Time
		ratio     float64
		want      bool
	}{
		{"zero notBefore", time.Time{}, now.Add(90 * day), 1.0 / 3, true},
		{"zero notAfter", now, time.Time{}, 1.0 / 3, true},
		{"inverted", now.Add(day), now, 1.0 / 3, true},
		{"expired", now.Add(-60 * day), now.Add(-time.Hour), 1.0 / 3, true},
		// 寿命恒为 90 天、1/3 窗口（=30 天）：剩 29 天进入窗口，剩 31 天未进入
		{"inside window", now.Add(-61 * day), now.Add(29 * day), 1.0 / 3, true},
		{"outside window", now.Add(-59 * day), now.Add(31 * day), 1.0 / 3, false},
		{"window boundary inclusive", now.Add(-60 * day), now.Add(30 * day), 1.0 / 3, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsRenewal(c.notBefore, c.notAfter, c.ratio); got != c.want {
				t.Fatalf("needsRenewal() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRetryDelay(t *testing.T) {
	const base = 5 * time.Minute
	const max = 12 * time.Hour
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, max},
		{1, 5 * time.Minute},
		{2, 10 * time.Minute},
		{4, 40 * time.Minute},
		{8, 640 * time.Minute},
		{9, max}, // 1280m 封顶
		{20, max},
	}
	for _, c := range cases {
		if got := retryDelay(c.failures, base, max); got != c.want {
			t.Fatalf("retryDelay(%d) = %v, want %v", c.failures, got, c.want)
		}
	}
}

func TestNewManagerValidation(t *testing.T) {
	tmp := t.TempDir()
	validCert := Cert{Domain: "a.example.com", CertFile: filepath.Join(tmp, "a.crt"), KeyFile: filepath.Join(tmp, "a.key")}
	cases := []struct {
		name string
		cfg  Config
	}{
		{"no certs", Config{}},
		{"empty domain", Config{Certs: []Cert{{}}}},
		{"wildcard domain", Config{Certs: []Cert{{Domain: "*.example.com"}}}},
		{"ip literal", Config{Certs: []Cert{{Domain: "10.0.0.1"}}}},
		{"duplicate domain", Config{Certs: []Cert{validCert, validCert}}},
		{"ratio too large", Config{Certs: []Cert{validCert}, RenewalWindowRatio: 1.0}},
		{"ratio negative", Config{Certs: []Cert{validCert}, RenewalWindowRatio: -0.1}},
		{"eab bad key", Config{Certs: []Cert{validCert}, EAB: &EAB{KID: "kid", Key: "!!!not-base64url"}}},
		{"eab empty kid", Config{Certs: []Cert{validCert}, EAB: &EAB{Key: "aGVsbG8"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewManager(c.cfg); err == nil {
				t.Fatal("NewManager() expected error, got nil")
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	m, err := NewManager(Config{Certs: []Cert{{Domain: "Example.COM!"}}})
	if err != nil {
		t.Fatal(err)
	}
	if m.cfg.DirectoryURL != acme.LetsEncryptURL {
		t.Fatalf("DirectoryURL = %s, want %s", m.cfg.DirectoryURL, acme.LetsEncryptURL)
	}
	if m.cfg.RenewalWindowRatio != 1.0/3.0 {
		t.Fatalf("RenewalWindowRatio = %v, want 1/3", m.cfg.RenewalWindowRatio)
	}
	if m.cfg.CheckInterval != 12*time.Hour {
		t.Fatalf("CheckInterval = %v, want 12h", m.cfg.CheckInterval)
	}
	st := m.certs["Example.COM!"]
	// 域名保留原样（SNI 分派 key），文件路径走 sanitize
	wantCert := filepath.Join("resources", "acme", "example.com_.crt")
	if st.certFile != wantCert {
		t.Fatalf("certFile = %s, want %s", st.certFile, wantCert)
	}
	if st.keyFile != filepath.Join("resources", "acme", "example.com_.key") {
		t.Fatalf("keyFile = %s", st.keyFile)
	}
	if m.acctFile != filepath.Join("resources", "acme", "account.key") {
		t.Fatalf("acctFile = %s", m.acctFile)
	}
	// 绝对路径原样使用
	m2, err := NewManager(Config{Certs: []Cert{{Domain: "x.example.com", CertFile: "/abs/x.crt", KeyFile: "/abs/x.key"}}})
	if err != nil {
		t.Fatal(err)
	}
	if st2 := m2.certs["x.example.com"]; st2.certFile != "/abs/x.crt" || st2.keyFile != "/abs/x.key" {
		t.Fatalf("absolute path not preserved: %s %s", st2.certFile, st2.keyFile)
	}
}

func newTestManager(t *testing.T, domains ...string) *Manager {
	t.Helper()
	tmp := t.TempDir()
	certs := make([]Cert, 0, len(domains))
	for _, d := range domains {
		certs = append(certs, Cert{
			Domain:   d,
			CertFile: filepath.Join(tmp, d+".crt"),
			KeyFile:  filepath.Join(tmp, d+".key"),
		})
	}
	m, err := NewManager(Config{
		Certs:          certs,
		AccountKeyFile: filepath.Join(tmp, "account.key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGetCertificate(t *testing.T) {
	m := newTestManager(t, "a.example.com", "b.example.com")
	dummy := &tls.Certificate{Leaf: &x509.Certificate{SerialNumber: big.NewInt(1)}}
	m.certs["a.example.com"].certPtr.Store(dummy)

	cases := []struct {
		name       string
		serverName string
		protos     []string
		wantCert   *tls.Certificate
		wantErr    bool
	}{
		{"match", "a.example.com", []string{"h2", "http/1.1"}, dummy, false},
		{"unconfigured domain", "other.com", []string{"h2"}, nil, false},
		{"empty sni", "", nil, nil, false},
		{"configured but not issued", "b.example.com", []string{"h2"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hello := &tls.ClientHelloInfo{ServerName: c.serverName, SupportedProtos: c.protos}
			cert, err := m.GetCertificate(hello)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if cert != c.wantCert {
				t.Fatalf("cert = %p, want %p", cert, c.wantCert)
			}
		})
	}

	t.Run("alpn challenge served for pending domain", func(t *testing.T) {
		challengeCert := &tls.Certificate{}
		m.setPending(&pendingChallenge{domain: "a.example.com", tlsCert: challengeCert})
		defer m.clearPending()
		hello := &tls.ClientHelloInfo{ServerName: "a.example.com", SupportedProtos: []string{acme.ALPNProto}}
		cert, err := m.GetCertificate(hello)
		if err != nil {
			t.Fatal(err)
		}
		if cert != challengeCert {
			t.Fatal("expected challenge certificate")
		}
	})

	t.Run("alpn challenge not served for other domain", func(t *testing.T) {
		hello := &tls.ClientHelloInfo{ServerName: "other.com", SupportedProtos: []string{acme.ALPNProto}}
		cert, err := m.GetCertificate(hello)
		if err != nil || cert != nil {
			t.Fatalf("cert = %v, err = %v, want nil, nil", cert, err)
		}
	})

	t.Run("alpn challenge without pending", func(t *testing.T) {
		hello := &tls.ClientHelloInfo{ServerName: "a.example.com", SupportedProtos: []string{acme.ALPNProto}}
		cert, err := m.GetCertificate(hello)
		if err != nil || cert != nil {
			t.Fatalf("cert = %v, err = %v, want nil, nil", cert, err)
		}
	})
}

func TestHTTP01Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newTestManager(t, "a.example.com")
	m.setPending(&pendingChallenge{token: "tok123", keyAuth: "tok123.thumbprint"})

	router := gin.New()
	router.Use(m.HTTP01Handler())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "root") })

	// 命中挑战 token
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok123", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "tok123.thumbprint" {
		t.Fatalf("code = %d, body = %q", w.Code, w.Body.String())
	}
	// token 不匹配 → 透传
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/other", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404 (passthrough)", w.Code)
	}
	// 非挑战路径 → 透传
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "root" {
		t.Fatalf("code = %d, body = %q", w.Code, w.Body.String())
	}
}
