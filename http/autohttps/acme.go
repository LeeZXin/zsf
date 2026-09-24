package autohttps

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/LeeZXin/zsf/logger"

	"golang.org/x/crypto/acme"
)

// ensureRegistered 注册/恢复 ACME 账号。同一账号私钥重复注册时 CA 返回
// ErrAccountAlreadyExists 但 KID 已缓存在 client 内，视为成功。
func (m *Manager) ensureRegistered(ctx context.Context) error {
	if m.registered {
		return nil
	}
	acct := &acme.Account{}
	if m.cfg.Email != "" {
		acct.Contact = []string{"mailto:" + m.cfg.Email}
	}
	if m.cfg.EAB != nil {
		key, err := decodeEABKey(m.cfg.EAB.Key)
		if err != nil {
			return fmt.Errorf("decoding eab key: %w", err)
		}
		acct.ExternalAccountBinding = &acme.ExternalAccountBinding{
			KID: m.cfg.EAB.KID,
			Key: key,
		}
	}
	if _, err := m.client.Register(ctx, acct, acme.AcceptTOS); err != nil {
		if errors.Is(err, acme.ErrAccountAlreadyExists) {
			m.registered = true
			return nil
		}
		return fmt.Errorf("registering account: %w", err)
	}
	m.registered = true
	return nil
}

// obtain 为 st 对应的域名执行一次完整 ACME 订单：授权验证 → 签发 → 落盘 → 热替换内存。
//
// 并发纪律：整个函数在网络 IO 期间绝不持有 m.mu（TLS-ALPN 验证需要握手协程
// 在续期进行中读到挑战证书，持锁执行会死锁——certmagic maintain.go 的教训）。
// pending 只在挑战开始/结束时短暂置入/清除。
func (m *Manager) obtain(ctx context.Context, st *certState) error {
	if err := m.ensureRegistered(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, m.cfg.OrderTimeout)
	defer cancel()

	domain := st.domain
	order, err := m.client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return fmt.Errorf("creating order: %w", err)
	}
	// 订单级清理，覆盖授权验证中所有失败路径
	defer m.clearPending()

	// 逐个授权验证（单域名订单通常只有一个）
	for _, authzURL := range order.AuthzURLs {
		authz, err := m.client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("getting authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue // 已有授权，跳过
		}
		chal := pickChallenge(authz.Challenges)
		if chal == nil {
			return fmt.Errorf("authorization offers no tls-alpn-01/http-01 challenge")
		}
		if err := m.solveChallenge(ctx, domain, authz, chal); err != nil {
			return err
		}
	}

	// 授权齐备后订单进入 ready，再 finalize
	if _, err := m.client.WaitOrder(ctx, order.URI); err != nil {
		return fmt.Errorf("waiting for order: %w", err)
	}

	// 私钥：与现有证书匹配则复用（续期不轮换，规避 cert/key 双文件写序的崩溃窗口），
	// 否则生成新私钥（首次签发）。
	key, keyIsNew, err := loadOrCreateCertKey(st)
	if err != nil {
		return err
	}
	csrDER, err := makeCSR(key, domain)
	if err != nil {
		return fmt.Errorf("generating CSR: %w", err)
	}
	chain, _, err := m.client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return fmt.Errorf("finalizing order: %w", err)
	}
	cert, err := buildCertificate(chain, key)
	if err != nil {
		return fmt.Errorf("building certificate: %w", err)
	}

	// 落盘。失败不放弃内存采纳（可用性优先）：certPtr 先换新，syncToDisk 下一轮补写。
	if err := writeChainPEM(st.certFile, chain, 0o644); err != nil {
		logger.Logger.Error().Err(err).Msgf("autohttps: %s write cert file failed", domain)
	}
	if keyIsNew {
		if err := writeKeyPEM(st.keyFile, key, 0o600); err != nil {
			logger.Logger.Error().Err(err).Msgf("autohttps: %s write key file failed", domain)
		}
	}
	st.certPtr.Store(cert)
	st.leaf.Store(leafMetaFrom(cert.Leaf))
	if fi, err := os.Stat(st.certFile); err == nil {
		st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
	}
	logger.Logger.Info().Msgf("autohttps: %s certificate issued, expires %s",
		domain, cert.Leaf.NotAfter.Format(time.RFC3339))
	return nil
}

// solveChallenge 求解单个挑战：预生成挑战证书/keyAuth 置入 pending，
// Accept 后等待 CA 验证（期间 pending 必须存活），完成后清除。
func (m *Manager) solveChallenge(ctx context.Context, domain string, authz *acme.Authorization, chal *acme.Challenge) error {
	p := &pendingChallenge{domain: domain, token: chal.Token}
	switch chal.Type {
	case "tls-alpn-01":
		cert, err := m.client.TLSALPN01ChallengeCert(chal.Token, domain)
		if err != nil {
			return fmt.Errorf("generating tls-alpn challenge cert: %w", err)
		}
		p.tlsCert = &cert
	case "http-01":
		keyAuth, err := m.client.HTTP01ChallengeResponse(chal.Token)
		if err != nil {
			return fmt.Errorf("generating http-01 key authorization: %w", err)
		}
		p.keyAuth = keyAuth
	default:
		return fmt.Errorf("unsupported challenge type %q", chal.Type)
	}
	m.setPending(p)
	defer m.clearPending()
	if _, err := m.client.Accept(ctx, chal); err != nil {
		return fmt.Errorf("accepting challenge: %w", err)
	}
	if _, err := m.client.WaitAuthorization(ctx, authz.URI); err != nil {
		return fmt.Errorf("waiting for authorization: %w", err)
	}
	return nil
}

// pickChallenge 按偏好选择挑战：优先 tls-alpn-01（零配置），次选 http-01。
func pickChallenge(chals []*acme.Challenge) *acme.Challenge {
	var http01 *acme.Challenge
	for _, c := range chals {
		switch c.Type {
		case "tls-alpn-01":
			return c
		case "http-01":
			if http01 == nil {
				http01 = c
			}
		}
	}
	return http01
}

// loadOrCreateCertKey 返回用于 CSR 的私钥：KeyFile 存在且与当前证书公钥匹配
// 则复用（续期沿用旧私钥）；否则生成新私钥并标记 keyIsNew（成功后落盘）。
func loadOrCreateCertKey(st *certState) (key crypto.Signer, keyIsNew bool, err error) {
	keyPEM, err := os.ReadFile(st.keyFile)
	if err == nil {
		if key, err = pemDecodePrivateKey(keyPEM); err == nil {
			if cur := st.certPtr.Load(); cur != nil && cur.Leaf != nil {
				if publicKeysEqual(cur.Leaf.PublicKey, key.Public()) {
					return key, false, nil
				}
			}
		}
		// 解析失败或不匹配：换新私钥重新签发
		logger.Logger.Error().Err(err).Msgf("autohttps: %s existing key unusable, generating new one", st.domain)
	}
	key, err = newCertKey()
	if err != nil {
		return nil, false, err
	}
	return key, true, nil
}

// makeCSR 生成单域名 CSR。
func makeCSR(key crypto.Signer, domain string) ([]byte, error) {
	return x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		DNSNames: []string{domain},
	}, key)
}

// buildCertificate 组装服务端证书：预解析叶子（避免握手路径重复解析），
// 链保留完整 DER（含签发者）。返回后对象不可变。
func buildCertificate(chain [][]byte, key crypto.Signer) (*tls.Certificate, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty certificate chain")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: chain,
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}

// publicKeysEqual 比较两个公钥是否相同。
func publicKeysEqual(a, b crypto.PublicKey) bool {
	ad, err1 := x509.MarshalPKIXPublicKey(a)
	bd, err2 := x509.MarshalPKIXPublicKey(b)
	return err1 == nil && err2 == nil && bytes.Equal(ad, bd)
}
