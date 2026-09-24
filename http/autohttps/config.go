package autohttps

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeeZXin/zsf/constants"

	"golang.org/x/crypto/acme"
)

// 维护策略默认值（参考 certmagic maintain.go 的 DefaultRenewCheckInterval /
// DefaultRenewalWindowRatio，按单机低频场景放宽检查间隔）。
const (
	// defaultRenewalWindowRatio 是证书寿命进入续期窗口的比例：剩余寿命 ≤ 1/3 时开始续期。
	// 90 天证书约提前 30 天尝试。
	defaultRenewalWindowRatio = 1.0 / 3.0
	// defaultCheckInterval 是维护循环的常规检查间隔。续期窗口通常长达数周，12h 足够。
	defaultCheckInterval = 12 * time.Hour
	// defaultOrderTimeout 是单次 ACME 订单（含挑战验证轮询）的全程超时。
	defaultOrderTimeout = 15 * time.Minute
	// retryBaseDelay 是续期失败后指数退避的起步间隔（5m → 10m → 20m → … 封顶 CheckInterval）。
	retryBaseDelay = 5 * time.Minute
	// defaultAccountKeyFile 是所有证书共享的 ACME 账号私钥默认路径（相对 resources 目录）。
	defaultAccountKeyFile = "acme/account.key"
	// userAgent 是 ACME 请求的 User-Agent。
	userAgent = "zsf-autohttps"
)

// Cert 描述一个待管理的证书：域名 + 证书/私钥落盘路径。
type Cert struct {
	// Domain 是证书域名（单个完整域名，不支持通配符与 IP——两者分别需要 DNS-01
	// 与 RFC 8738 验证路径，本包不支持）。
	Domain string `json:"domain"`
	// CertFile 是证书文件路径（相对 resources 目录，绝对路径原样使用）；
	// 空则默认 acme/<sanitize(domain)>.crt。文件内容为完整链 PEM。
	CertFile string `json:"certFile,omitempty"`
	// KeyFile 是私钥文件路径；空则默认 acme/<sanitize(domain)>.key。
	KeyFile string `json:"keyFile,omitempty"`
}

// EAB 是内部 ACME CA 的外部账户绑定（externalAccountBinding）。
type EAB struct {
	// KID 是 CA 发放的 key identifier。
	KID string `json:"kid"`
	// Key 是 base64url（RawURLEncoding）编码的 HS256 对称密钥。
	// 注意：x/crypto 的 ExternalAccountBinding.Key 是密钥原始字节（[]byte），不是 crypto.Signer。
	Key string `json:"key"`
}

// Config 是 Manager 的配置。全部证书共享一个 ACME 账号。
type Config struct {
	// Certs 是待管理的证书列表，必填非空，列表内域名不得重复。
	Certs []Cert
	// Email 是 ACME 账号联系邮箱（注册时作为 contact），可选。
	Email string
	// AccountKeyFile 是 ACME 账号私钥路径；空则默认 acme/account.key。
	// 不存在时自动生成 EC P-256 并持久化（0600）；存在但损坏会启动失败——
	// 账号私钥一旦更换等于换新账号，需要人工介入。
	AccountKeyFile string
	// DirectoryURL 是 ACME CA 目录地址，默认 Let's Encrypt production。
	DirectoryURL string
	// EAB 是外部账户绑定，接入内部 ACME CA 时按需配置。
	EAB *EAB
	// HTTPClient 是 ACME 请求使用的客户端（默认 http.DefaultClient）。
	// 可注入以定制代理/TLS 信任，测试也用它接入 stub CA。
	HTTPClient *http.Client
	// RenewalWindowRatio 是续期窗口占证书寿命的比例，默认 1/3。
	RenewalWindowRatio float64
	// CheckInterval 是维护循环常规检查间隔，默认 12h。
	CheckInterval time.Duration
	// OrderTimeout 是单次 ACME 订单全程超时，默认 15m。
	OrderTimeout time.Duration
}

// resolvePath 解析证书/私钥文件路径：相对路径按 resources 目录惯例解析，
// 绝对路径原样使用（与 zsf/http/server 的 EnableHttps 一致，但容忍绝对路径）。
func resolvePath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(constants.ResourcesDir, p)
}

// sanitizeDomain 将域名中非 [a-z0-9.-] 的字符替换为 _，用于生成默认文件名。
func sanitizeDomain(domain string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(domain) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// withDefaults 返回应用默认值后的配置副本，不修改入参。
func (c Config) withDefaults() Config {
	if c.DirectoryURL == "" {
		c.DirectoryURL = acme.LetsEncryptURL
	}
	if c.AccountKeyFile == "" {
		c.AccountKeyFile = defaultAccountKeyFile
	}
	if c.RenewalWindowRatio == 0 {
		c.RenewalWindowRatio = defaultRenewalWindowRatio
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = defaultCheckInterval
	}
	if c.OrderTimeout <= 0 {
		c.OrderTimeout = defaultOrderTimeout
	}
	for i := range c.Certs {
		if c.Certs[i].CertFile == "" {
			c.Certs[i].CertFile = "acme/" + sanitizeDomain(c.Certs[i].Domain) + ".crt"
		}
		if c.Certs[i].KeyFile == "" {
			c.Certs[i].KeyFile = "acme/" + sanitizeDomain(c.Certs[i].Domain) + ".key"
		}
		c.Certs[i].CertFile = resolvePath(c.Certs[i].CertFile)
		c.Certs[i].KeyFile = resolvePath(c.Certs[i].KeyFile)
	}
	c.AccountKeyFile = resolvePath(c.AccountKeyFile)
	return c
}

// validate 校验配置合法性，返回 error 由调用方决定如何处理（NewManager 直接返回）。
func (c Config) validate() error {
	if len(c.Certs) == 0 {
		return fmt.Errorf("autohttps: no certificates configured")
	}
	seen := make(map[string]struct{}, len(c.Certs))
	for _, cert := range c.Certs {
		if err := validateDomain(cert.Domain); err != nil {
			return err
		}
		if _, dup := seen[cert.Domain]; dup {
			return fmt.Errorf("autohttps: duplicate domain in certs: %s", cert.Domain)
		}
		seen[cert.Domain] = struct{}{}
	}
	if c.RenewalWindowRatio <= 0 || c.RenewalWindowRatio >= 1 {
		return fmt.Errorf("autohttps: renewal window ratio must be in (0, 1), got %v", c.RenewalWindowRatio)
	}
	if c.EAB != nil {
		if c.EAB.KID == "" {
			return fmt.Errorf("autohttps: eab kid is empty")
		}
		if _, err := decodeEABKey(c.EAB.Key); err != nil {
			return fmt.Errorf("autohttps: eab key: %w", err)
		}
	}
	return nil
}

// validateDomain 拒绝空域名、通配符（需要 DNS-01，不支持）与 IP 字面量
// （需要 RFC 8738 的 ip identifier，不支持）。
func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("autohttps: cert domain is empty")
	}
	if strings.HasPrefix(domain, "*.") {
		return fmt.Errorf("autohttps: wildcard domain %s requires DNS-01 challenge, not supported", domain)
	}
	if net.ParseIP(domain) != nil {
		return fmt.Errorf("autohttps: IP address %s requires RFC 8738 validation, not supported", domain)
	}
	return nil
}

// decodeEABKey 解码 base64url 编码的 EAB 对称密钥，返回原始字节。
func decodeEABKey(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base64url encoding: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("decoded key is empty")
	}
	return b, nil
}
