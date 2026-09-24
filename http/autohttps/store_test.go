package autohttps

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeTestCert 生成自签名测试证书（链仅含叶子）。
func makeTestCert(t *testing.T, domain string, notBefore, notAfter time.Time) (*tls.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: domain},
		DNSNames:              []string{domain},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, key
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	// 父目录由上层（OnApplicationStart）负责创建，writeAtomic 只保证文件级原子性
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "file.txt")
	if err := writeAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("content = %q, err = %v", got, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
	// 无临时文件残留
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp file leftover: %s", e.Name())
		}
	}
	// 覆盖写同样原子
	if err := writeAtomic(path, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "world" {
		t.Fatalf("content = %q after overwrite", got)
	}
}

func TestCertChainRoundtrip(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "a.crt")
	keyFile := filepath.Join(dir, "a.key")

	now := time.Now()
	cert, key := makeTestCert(t, "a.example.com", now.Add(-time.Hour), now.Add(24*time.Hour))
	if err := writeChainPEM(certFile, cert.Certificate, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeKeyPEM(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadCertChain(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Leaf.SerialNumber.Cmp(cert.Leaf.SerialNumber) != 0 {
		t.Fatalf("serial mismatch: %s vs %s", loaded.Leaf.SerialNumber, cert.Leaf.SerialNumber)
	}
	if !publicKeysEqual(loaded.PrivateKey.(*ecdsa.PrivateKey).Public(), key.Public()) {
		t.Fatal("private key mismatch")
	}

	// 链 PEM：leaf + 伪造签发者两个 CERTIFICATE 块，全部收录且不影响加载
	chainPEM := make([]byte, 0, 2048)
	for _, der := range append(cert.Certificate, cert.Certificate[0]) {
		chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyPEM, _ := os.ReadFile(keyFile)
	parsed, err := tls.X509KeyPair(chainPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Certificate) != 2 {
		t.Fatalf("chain blocks = %d, want 2", len(parsed.Certificate))
	}
}

func TestLoadAccountKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acct.key")
	k1, err := loadAccountKey(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
	// 二次加载复用同一把 key（公钥一致）
	k2, err := loadAccountKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !publicKeysEqual(k1.Public(), k2.Public()) {
		t.Fatal("account key not reused")
	}
	// 损坏文件报错
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAccountKey(path); err == nil {
		t.Fatal("expected error for corrupt account key")
	}
}

func TestAdoptFromDisk(t *testing.T) {
	m := newTestManager(t, "a.example.com")
	st := m.certs["a.example.com"]

	now := time.Now()
	certA, keyA := makeTestCert(t, "a.example.com", now.Add(-time.Hour), now.Add(24*time.Hour))
	if err := writeChainPEM(st.certFile, certA.Certificate, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeKeyPEM(st.keyFile, keyA, 0o600); err != nil {
		t.Fatal(err)
	}
	// 磁盘内容变化 → 采纳
	m.adoptFromDisk(st)
	if got := st.certPtr.Load(); got == nil || got.Leaf.SerialNumber.Cmp(certA.Leaf.SerialNumber) != 0 {
		t.Fatal("cert A not adopted")
	}
	firstPtr := st.certPtr.Load()
	// stat 未变 → 不重复加载（指针不变）
	m.adoptFromDisk(st)
	if st.certPtr.Load() != firstPtr {
		t.Fatal("adoptFromDisk reloaded unchanged cert")
	}
	// 外部替换为新证书（cert/key 成对替换，如 certbot）→ 采纳
	certB, keyB := makeTestCert(t, "a.example.com", now.Add(-time.Hour), now.Add(48*time.Hour))
	if err := writeChainPEM(st.certFile, certB.Certificate, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeKeyPEM(st.keyFile, keyB, 0o600); err != nil {
		t.Fatal(err)
	}
	m.adoptFromDisk(st)
	if got := st.certPtr.Load(); got.Leaf.SerialNumber.Cmp(certB.Leaf.SerialNumber) != 0 {
		t.Fatal("cert B not adopted")
	}
	// 磁盘证书损坏 → 保持现有内存证书
	if err := os.WriteFile(st.certFile, []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.adoptFromDisk(st)
	if got := st.certPtr.Load(); got.Leaf.SerialNumber.Cmp(certB.Leaf.SerialNumber) != 0 {
		t.Fatal("memory cert replaced by unloadable disk cert")
	}
}

func TestSyncToDisk(t *testing.T) {
	m := newTestManager(t, "a.example.com")
	st := m.certs["a.example.com"]

	// 磁盘与内存不一致（内存有、磁盘无）→ 补写
	now := time.Now()
	cert, _ := makeTestCert(t, "a.example.com", now.Add(-time.Hour), now.Add(24*time.Hour))
	st.certPtr.Store(cert)
	m.syncToDisk(st)
	if _, err := os.Stat(st.certFile); err != nil {
		t.Fatalf("cert file not synced: %v", err)
	}
	fi, err := os.Stat(st.keyFile)
	if err != nil {
		t.Fatalf("key file not synced: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v, want 0600", fi.Mode().Perm())
	}
	// 磁盘已一致 → 不重写（stat 稳定）
	before, _ := os.Stat(st.certFile)
	m.syncToDisk(st)
	after, _ := os.Stat(st.certFile)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("syncToDisk rewrote identical cert")
	}
}
