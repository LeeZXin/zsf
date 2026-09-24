package autohttps

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// writeAtomic 原子写文件：同目录临时文件写完 fsync 后 rename，
// 保证读者永远看不到半截内容（参考 certmagic internal/atomicfile）。
// 失败时清理临时文件。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// writeChainPEM 将 DER 证书链按 leaf 在前编码为 PEM 并原子写入。
func writeChainPEM(path string, chain [][]byte, mode os.FileMode) error {
	var buf []byte
	for _, der := range chain {
		buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	return writeAtomic(path, buf, mode)
}

// writeKeyPEM 将私钥以 PKCS8 PEM 编码原子写入（0600）。
func writeKeyPEM(path string, key crypto.Signer, mode os.FileMode) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling private key: %w", err)
	}
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), mode)
}

// loadCertChain 从磁盘加载证书与私钥。证书文件允许包含完整链 PEM
// （tls.X509KeyPair 按序收录全部 CERTIFICATE 块，仅校验首块与私钥匹配）。
func loadCertChain(certFile, keyFile string) (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading key pair: %w", err)
	}
	return &cert, nil
}

// loadAccountKey 加载 ACME 账号私钥（PKCS8 或 EC PRIVATE KEY），
// 不存在时生成 EC P-256 并持久化（0600）。
func loadAccountKey(path string) (crypto.Signer, error) {
	keyPEM, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating account key: %w", err)
		}
		if err := writeKeyPEM(path, key, 0o600); err != nil {
			return nil, fmt.Errorf("saving account key: %w", err)
		}
		return key, nil
	}
	key, err := pemDecodePrivateKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parsing account key %s: %w", path, err)
	}
	return key, nil
}

// pemDecodePrivateKey 解析 PKCS8（PRIVATE KEY）或 SEC1（EC PRIVATE KEY）格式的私钥。
func pemDecodePrivateKey(data []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("key of type %T is not a signer", key)
		}
		return signer, nil
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}

// newCertKey 生成 EC P-256 证书私钥。
func newCertKey() (crypto.Signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating certificate key: %w", err)
	}
	return key, nil
}
