package autohttps

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubCA 是 RFC 8555 的最小实现，供离线集成测试：
//   - 每个响应带 Replay-Nonce 头（acme 客户端从响应缓存 nonce），另实现 HEAD /nonce 兜底；
//   - new-account / new-order / authz 都回 Location 头（客户端据此取 KID/Order.URI/Authz.URI）；
//   - Accept 挑战后立即置 authz valid（不实际 dial 443，验证的是客户端完整订单流程）；
//   - finalize 用自造 CA 对订单域名签发叶子证书，返回 leaf+CA 完整链 PEM。
//
// 状态以 URL 路径为 key（/order/N、/authz/N、/chal/N/i、/finalize/N、/cert/N），
// 客户端请求的都是绝对 URL，用 r.URL.Path 即可定位。
type stubCA struct {
	mu       sync.Mutex
	nonce    atomic.Int64
	orders   atomic.Int64
	domains  []string // 按订单序号分配域名；超出则复用最后一个（续期场景）
	lifetime time.Duration
	chals    []string // 提供的挑战类型

	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey

	ordersByID  map[string]*stubOrder // key: 订单序号
	authzsByID  map[string]*stubAuthz // key: 订单序号
	chalToAuthz map[string]string     // /chal/N/i → 订单序号
}

type stubOrder struct {
	domain      string
	status      string
	finalizeURL string
	certURL     string
	certPEM     []byte
	authzURL    string
}

type stubAuthz struct {
	domain string
	valid  bool
	chals  []stubChal
}

type stubChal struct {
	typ   string
	url   string
	token string
}

func newStubCA(t *testing.T, domains []string, chals []string, lifetime time.Duration) *stubCA {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "stub acme ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	return &stubCA{
		domains:     domains,
		lifetime:    lifetime,
		chals:       chals,
		caCert:      caCert,
		caKey:       caKey,
		ordersByID:  make(map[string]*stubOrder),
		authzsByID:  make(map[string]*stubAuthz),
		chalToAuthz: make(map[string]string),
	}
}

func (s *stubCA) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Replay-Nonce", fmt.Sprintf("nonce-%d", s.nonce.Add(1)))
	base := "https://" + r.Host // directory 里的 URL 必须是绝对地址
	switch {
	case r.URL.Path == "/":
		writeJSON(w, http.StatusOK, map[string]any{
			"newNonce":   base + "/nonce",
			"newAccount": base + "/new-account",
			"newOrder":   base + "/new-order",
		})
	case r.URL.Path == "/nonce" && r.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case r.URL.Path == "/new-account":
		w.Header().Set("Location", base+"/acct/1")
		writeJSON(w, http.StatusCreated, map[string]any{"status": "valid"})
	case r.URL.Path == "/new-order":
		s.serveNewOrder(w, base)
	case strings.HasPrefix(r.URL.Path, "/authz/"):
		s.serveAuthz(w, base, r.URL.Path)
	case strings.HasPrefix(r.URL.Path, "/chal/"):
		s.serveChal(w, r.URL.Path)
	case strings.HasPrefix(r.URL.Path, "/order/"):
		s.serveOrder(w, r.URL.Path)
	case strings.HasPrefix(r.URL.Path, "/finalize/"):
		s.serveFinalize(w, r.URL.Path)
	case strings.HasPrefix(r.URL.Path, "/cert/"):
		s.serveCert(w, r.URL.Path)
	default:
		http.NotFound(w, r)
	}
}

func (s *stubCA) serveNewOrder(w http.ResponseWriter, base string) {
	n := s.orders.Add(1)
	domain := s.domains[len(s.domains)-1]
	if int(n) <= len(s.domains) {
		domain = s.domains[n-1]
	}
	id := strconv.FormatInt(n, 10)
	authzURL := base + "/authz/" + id
	chals := make([]stubChal, 0, len(s.chals))
	for i, typ := range s.chals {
		chalURL := fmt.Sprintf("%s/chal/%d/%d", base, n, i)
		chals = append(chals, stubChal{typ: typ, url: chalURL, token: fmt.Sprintf("tok-%d-%d", n, i)})
	}
	s.mu.Lock()
	s.authzsByID[id] = &stubAuthz{domain: domain, chals: chals}
	for _, c := range chals {
		s.chalToAuthz[strings.TrimPrefix(c.url, base)] = id
	}
	s.ordersByID[id] = &stubOrder{
		domain:      domain,
		status:      "pending",
		finalizeURL: base + "/finalize/" + id,
		certURL:     base + "/cert/" + id,
		authzURL:    authzURL,
	}
	s.mu.Unlock()
	w.Header().Set("Location", base+"/order/"+id)
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":         "pending",
		"identifiers":    []map[string]string{{"type": "dns", "value": domain}},
		"authorizations": []string{authzURL},
		"finalize":       base + "/finalize/" + id,
	})
}

func (s *stubCA) serveAuthz(w http.ResponseWriter, base, path string) {
	s.mu.Lock()
	authz, ok := s.authzsByID[strings.TrimPrefix(path, "/authz/")]
	if !ok {
		s.mu.Unlock()
		http.NotFound(w, nil)
		return
	}
	status := "pending"
	if authz.valid {
		status = "valid"
	}
	chals := make([]map[string]string, 0, len(authz.chals))
	for _, c := range authz.chals {
		chals = append(chals, map[string]string{
			"type": c.typ, "url": c.url, "token": c.token, "status": "pending",
		})
	}
	domain := authz.domain
	s.mu.Unlock()
	w.Header().Set("Location", base+path)
	writeJSON(w, http.StatusOK, map[string]any{
		"identifier": map[string]string{"type": "dns", "value": domain},
		"status":     status,
		"challenges": chals,
		"expires":    time.Now().Add(time.Hour).Format(time.RFC3339),
	})
}

func (s *stubCA) serveChal(w http.ResponseWriter, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Accept 即置 authz valid，不实际 dial 443
	if authzID, ok := s.chalToAuthz[path]; ok {
		if a := s.authzsByID[authzID]; a != nil {
			a.valid = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "valid"})
}

func (s *stubCA) serveOrder(w http.ResponseWriter, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.ordersByID[strings.TrimPrefix(path, "/order/")]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	if a := s.authzsByID[strings.TrimPrefix(pathOf(order.authzURL), "/authz/")]; a != nil && a.valid && order.status == "pending" {
		order.status = "ready"
	}
	resp := map[string]any{
		"status":         order.status,
		"identifiers":    []map[string]string{{"type": "dns", "value": order.domain}},
		"authorizations": []string{order.authzURL},
		"finalize":       order.finalizeURL,
	}
	if order.status == "valid" {
		resp["certificate"] = order.certURL
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *stubCA) serveFinalize(w http.ResponseWriter, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.ordersByID[strings.TrimPrefix(path, "/finalize/")]
	if !ok || order.status != "ready" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"type": "urn:ietf:params:acme:error:orderNotReady",
		})
		return
	}
	leaf := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: order.domain},
		DNSNames:              []string{order.domain},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(s.lifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, s.caCert, &leafKey.PublicKey, s.caKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	order.certPEM = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.caCert.Raw})...)
	order.status = "valid"
	writeJSON(w, http.StatusOK, map[string]any{"status": "valid", "certificate": order.certURL})
}

func (s *stubCA) serveCert(w http.ResponseWriter, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.ordersByID[strings.TrimPrefix(path, "/cert/")]
	if !ok || order.certPEM == nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "application/pem-certificate-chain")
	w.Write(order.certPEM)
}

// pathOf 取绝对 URL 的路径部分。
func pathOf(url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	}
	if i := strings.IndexByte(url, '/'); i >= 0 {
		return url[i:]
	}
	return "/"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func tlsHello(name string, protos []string) *tls.ClientHelloInfo {
	return &tls.ClientHelloInfo{ServerName: name, SupportedProtos: protos}
}

func newTestManagerWithCA(t *testing.T, ts *httptest.Server, domains []string, mutate func(*Config)) *Manager {
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
	cfg := Config{
		Certs:          certs,
		AccountKeyFile: filepath.Join(tmp, "account.key"),
		DirectoryURL:   ts.URL,
		HTTPClient:     ts.Client(),
		CheckInterval:  100 * time.Millisecond,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	m := NewManager(cfg)
	return m
}

// TestObtainMultipleCerts 全流程：多域名各自完成申请并落盘，SNI 分派正确。
func TestObtainMultipleCerts(t *testing.T) {
	s := newStubCA(t, []string{"a.example.com", "b.example.com"}, []string{"tls-alpn-01", "http-01"}, time.Hour)
	ts := httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(ts.Close)
	m := newTestManagerWithCA(t, ts, []string{"a.example.com", "b.example.com"}, nil)
	m.OnApplicationStart()
	m.AfterInitialize()
	t.Cleanup(m.OnApplicationShutdown)

	waitFor(t, 20*time.Second, func() bool {
		for _, d := range []string{"a.example.com", "b.example.com"} {
			if m.certs[d].certPtr.Load() == nil {
				return false
			}
		}
		return true
	})
	// 落盘与权限
	for _, d := range []string{"a.example.com", "b.example.com"} {
		st := m.certs[d]
		leaf := st.certPtr.Load().Leaf
		if leaf.DNSNames[0] != d {
			t.Fatalf("%s: leaf DNSNames = %v", d, leaf.DNSNames)
		}
		if fi, err := os.Stat(st.certFile); err != nil || fi.Mode().Perm() != 0o644 {
			t.Fatalf("%s: cert file mode/err = %v/%v", d, fi.Mode().Perm(), err)
		}
		if fi, err := os.Stat(st.keyFile); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: key file mode/err = %v/%v", d, fi.Mode().Perm(), err)
		}
	}
	// SNI 分派
	for _, d := range []string{"a.example.com", "b.example.com"} {
		cert, err := m.GetCertificate(tlsHello(d, nil))
		if err != nil || cert == nil || cert.Leaf.DNSNames[0] != d {
			t.Fatalf("%s: dispatch failed: cert=%v err=%v", d, cert, err)
		}
	}
	if got := s.orders.Load(); got != 2 {
		t.Fatalf("orders = %d, want 2", got)
	}
}

// TestRenewalShortLivedCert 短寿命证书 + 大窗口比例触发续期，旧证书热替换。
func TestRenewalShortLivedCert(t *testing.T) {
	s := newStubCA(t, []string{"a.example.com"}, []string{"tls-alpn-01"}, 10*time.Second)
	ts := httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(ts.Close)
	m := newTestManagerWithCA(t, ts, []string{"a.example.com"}, func(cfg *Config) {
		cfg.RenewalWindowRatio = 0.9 // 10s 寿命 → 窗口 9s，签发后 ~1s 即进入
	})
	m.OnApplicationStart()
	m.AfterInitialize()
	t.Cleanup(m.OnApplicationShutdown)

	st := m.certs["a.example.com"]
	waitFor(t, 20*time.Second, func() bool { return st.certPtr.Load() != nil })
	first := st.certPtr.Load().Leaf.SerialNumber
	waitFor(t, 20*time.Second, func() bool {
		cur := st.certPtr.Load()
		return cur != nil && cur.Leaf.SerialNumber.Cmp(first) != 0
	})
	if got := s.orders.Load(); got < 2 {
		t.Fatalf("orders = %d, want >= 2 (initial + renewal)", got)
	}
}

// TestObtainViaHTTP01 CA 只提供 http-01 挑战时流程走通。
func TestObtainViaHTTP01(t *testing.T) {
	s := newStubCA(t, []string{"a.example.com"}, []string{"http-01"}, time.Hour)
	ts := httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(ts.Close)
	m := newTestManagerWithCA(t, ts, []string{"a.example.com"}, nil)
	m.OnApplicationStart()
	m.AfterInitialize()
	t.Cleanup(m.OnApplicationShutdown)

	waitFor(t, 20*time.Second, func() bool { return m.certs["a.example.com"].certPtr.Load() != nil })
	if got := s.orders.Load(); got != 1 {
		t.Fatalf("orders = %d, want 1", got)
	}
}

// TestWriteFailureStillAdopts 落盘失败（证书路径被目录占用）不放弃内存采纳。
func TestWriteFailureStillAdopts(t *testing.T) {
	s := newStubCA(t, []string{"a.example.com"}, []string{"tls-alpn-01"}, time.Hour)
	ts := httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(ts.Close)
	m := newTestManagerWithCA(t, ts, []string{"a.example.com"}, nil)
	// 用目录占住证书文件路径，写盘必然失败
	if err := os.MkdirAll(m.certs["a.example.com"].certFile, 0o700); err != nil {
		t.Fatal(err)
	}
	m.OnApplicationStart()
	m.AfterInitialize()
	t.Cleanup(m.OnApplicationShutdown)

	waitFor(t, 20*time.Second, func() bool { return m.certs["a.example.com"].certPtr.Load() != nil })
	if got := s.orders.Load(); got != 1 {
		t.Fatalf("orders = %d, want 1 (no repeat renewals)", got)
	}
}
