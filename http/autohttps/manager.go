package autohttps

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LeeZXin/zsf/logger"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/acme"
)

// Manager 管理一组域名的 HTTPS 证书：按需通过 ACME 申请，到期前自动续期，
// 续期成功后热替换内存证书（经 GetCertificate 回调供 HTTP 服务使用）。
// 实现 lifecycle.Object，接入 zsf/lifecycle 后自动启停。
//
// 并发纪律（参考 certmagic 的锁设计教训）：
//   - 维护循环是唯一执行申请/续期/磁盘 IO 的 goroutine，网络 IO 期间绝不持锁；
//   - pending 挑战用短临界区 mu 保护（TLS-ALPN 验证需要握手协程在续期期间
//     读到挑战证书，若续期持锁执行会死锁）；
//   - 每个域名的证书经 atomic.Pointer 热替换，握手热路径零锁；
//   - certs map 启动时构建完成后只读，多 goroutine 并发读安全。
type Manager struct {
	cfg      Config
	order    []string              // 域名迭代顺序（配置顺序，保证日志/测试确定性）
	certs    map[string]*certState // key: domain，启动后只读
	acctFile string
	client   *acme.Client // 账号共享；OnApplicationStart 构造

	mu      sync.Mutex
	pending *pendingChallenge // 单槽：维护循环串行，任一时刻至多一个订单在途

	ctx     context.Context
	cancel  context.CancelFunc
	trigger chan struct{} // AfterInitialize 唤醒首轮
	wg      sync.WaitGroup

	// registered 标记 ACME 账号已注册（KID 已缓存在 client 内），
	// 仅维护 goroutine 读写，省去每次签发的注册往返。
	registered bool
}

// certState 是单个域名的证书运行时状态。
type certState struct {
	domain      string
	certFile    string
	keyFile     string
	certPtr     atomic.Pointer[tls.Certificate] // 热路径只读；swap 后对象不可变
	leaf        atomic.Pointer[leafMeta]        // 续期判定用叶子证书元信息
	diskModTime time.Time                       // 以下两项仅维护 goroutine 读写
	diskSize    int64
}

// leafMeta 是叶子证书的续期判定信息。
type leafMeta struct {
	serial    *big.Int
	notBefore time.Time
	notAfter  time.Time
}

// pendingChallenge 是进行中的 ACME 挑战状态。
// 创建后不可变，GetCertificate/HTTP01Handler 在短临界区内取指针后安全读取。
type pendingChallenge struct {
	domain  string           // tls-alpn-01 分派依据（SNI）
	token   string           // http-01 分派依据（URL 末段）
	keyAuth string           // http-01 预计算的 key authorization
	tlsCert *tls.Certificate // tls-alpn-01 预生成的挑战证书（可能 nil）
}

// NewManager 校验配置并创建 Manager（不产生副作用，文件/网络动作在 OnApplicationStart）。
// 返回 error 由调用方决定处理（通常配合 logger.Logger.Fatal() fail-fast）。
func NewManager(cfg Config) *Manager {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		logger.Logger.Fatal().Err(err).Msg("autohttps manager config validation failed")
	}
	m := &Manager{
		cfg:      cfg,
		acctFile: cfg.AccountKeyFile,
		certs:    make(map[string]*certState, len(cfg.Certs)),
		trigger:  make(chan struct{}, 1),
	}
	for _, cert := range cfg.Certs {
		m.order = append(m.order, cert.Domain)
		m.certs[cert.Domain] = &certState{
			domain:   cert.Domain,
			certFile: cert.CertFile,
			keyFile:  cert.KeyFile,
		}
	}
	return m
}

// Order 返回启动顺序：-100，先于 Order 0 的 HTTP 服务启动、逆序最后关停，
// 保证证书先就绪、服务排空期间证书仍可用。
func (m *Manager) Order() int {
	return -100
}

// OnApplicationStart 在 main goroutine 中执行，硬错误 Fatal fail-fast：
// 建目录、加载/生成 ACME 账号私钥、best-effort 加载磁盘证书、启动维护循环。
//
// 磁盘证书加载是 best-effort（缺失/损坏只记日志不 Fatal）：与严格 fail-fast
// 的取舍在于 autohttps 的定位是自愈——缺失的证书由维护循环签发补上，
// 若此处 Fatal 则进程反复启动失败反而需要人工救火。
func (m *Manager) OnApplicationStart() {
	// 建目录（证书/私钥/账号私钥所在目录）
	dirs := map[string]struct{}{}
	for _, st := range m.certs {
		dirs[filepath.Dir(st.certFile)] = struct{}{}
		dirs[filepath.Dir(st.keyFile)] = struct{}{}
	}
	dirs[filepath.Dir(m.acctFile)] = struct{}{}
	for dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			logger.Logger.Fatal().Err(err).Msgf("autohttps: create dir %s", dir)
		}
	}
	// ACME 账号私钥：存在但损坏必须 Fatal（换 key 等于换新账号，需人工介入）
	acctKey, err := loadAccountKey(m.acctFile)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("autohttps: load account key")
	}
	m.client = &acme.Client{
		Key:          acctKey,
		DirectoryURL: m.cfg.DirectoryURL,
		HTTPClient:   m.cfg.HTTPClient,
		UserAgent:    userAgent,
	}
	// 磁盘证书 best-effort 加载
	for _, domain := range m.order {
		st := m.certs[domain]
		cert, err := loadCertChain(st.certFile, st.keyFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				logger.Logger.Info().Msgf("autohttps: %s has no certificate yet, will obtain", domain)
			} else {
				logger.Logger.Error().Err(err).Msgf("autohttps: %s existing certificate unloadable, will obtain", domain)
			}
			continue
		}
		st.certPtr.Store(cert)
		st.leaf.Store(leafMetaFrom(cert.Leaf))
		if fi, statErr := os.Stat(st.certFile); statErr == nil {
			st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
		}
		logger.Logger.Info().Msgf("autohttps: %s loaded certificate expiring %s",
			domain, cert.Leaf.NotAfter.Format(time.RFC3339))
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.wg.Add(1)
	go m.maintainLoop()
}

// AfterInitialize 在所有对象启动完成后非阻塞触发首轮检查：
// 此时 80/443 监听已就绪，ACME 挑战可达。签发含轮询可能耗时数十秒，
// 不在 main goroutine 上同步执行。
func (m *Manager) AfterInitialize() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// OnApplicationShutdown 停止维护循环并等待在途 ACME 调用经 ctx 中断退出。
func (m *Manager) OnApplicationShutdown() {
	m.cancel()
	m.wg.Wait()
}

// GetCertificate 实现 tls.Config.GetCertificate 回调（热路径，无锁）：
//   - SNI 为本包管理的域名且 ALPN 为 acme-tls/1 → 返回进行中的挑战证书（TLS-ALPN-01 验证）；
//   - SNI 为已配置域名 → 返回当前证书（未签发时返回 error，握手以 internal_error 失败）；
//   - 其余（未配置域名、空 SNI）→ (nil, nil)，stdlib 会落到 unrecognized_name alert。
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	// TLS-ALPN-01：CA 校验器以 acme-tls/1 单协议 + 域名 SNI 发起握手
	if len(hello.SupportedProtos) == 1 && hello.SupportedProtos[0] == acme.ALPNProto {
		m.mu.Lock()
		p := m.pending
		m.mu.Unlock()
		if p != nil && p.domain == hello.ServerName && p.tlsCert != nil {
			return p.tlsCert, nil
		}
		return nil, nil
	}
	st := m.certs[hello.ServerName]
	if st == nil {
		return nil, nil
	}
	cert := st.certPtr.Load()
	if cert == nil {
		return nil, fmt.Errorf("autohttps: certificate for %s not issued yet", hello.ServerName)
	}
	return cert, nil
}

// HTTP01Handler 返回 HTTP-01 挑战的 gin 中间件：拦截 /.well-known/acme-challenge/<token>
// 且 token 属于进行中挑战的请求，返回 key authorization；否则透传。
// 必须注册在 80→443 redirect 等拦截中间件之前（gin 按注册顺序执行）。
func (m *Manager) HTTP01Handler() gin.HandlerFunc {
	const prefix = "/.well-known/acme-challenge/"
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, prefix) {
			c.Next()
			return
		}
		token := strings.TrimPrefix(path, prefix)
		m.mu.Lock()
		p := m.pending
		m.mu.Unlock()
		if p != nil && p.token == token && p.keyAuth != "" {
			c.String(http.StatusOK, p.keyAuth)
			c.Abort()
			return
		}
		c.Next()
	}
}

// maintainLoop 是唯一的维护 goroutine。首轮由 AfterInitialize 的 trigger 唤醒，
// 之后按成功间隔（CheckInterval）或失败退避间隔执行。
func (m *Manager) maintainLoop() {
	defer m.wg.Done()
	failures := 0
	timer := time.NewTimer(m.cfg.CheckInterval)
	defer timer.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.trigger:
		case <-timer.C:
		}
		if m.safeStep() {
			failures++
		} else {
			failures = 0
		}
		timer.Reset(retryDelay(failures, retryBaseDelay, m.cfg.CheckInterval))
	}
}

// safeStep 包装 step：panic 只记日志、不影响循环存活
// （后台 goroutine 里 logger Fatal 会 os.Exit，这里绝不 Fatal）。
func (m *Manager) safeStep() (failed bool) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error().Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("autohttps: maintenance step panic")
			failed = true
		}
	}()
	return m.step()
}

// step 对每个域名依次：采纳磁盘证书 → 补写未落盘的证书 → 按需申请/续期。
// 返回 true 表示本轮有域名申请失败（进入退避）。
func (m *Manager) step() (failed bool) {
	for _, domain := range m.order {
		st := m.certs[domain]
		m.adoptFromDisk(st)
		m.syncToDisk(st)
		if !m.needsObtain(st) {
			continue
		}
		if err := m.obtain(m.ctx, st); err != nil {
			logger.Logger.Error().Err(err).Msgf("autohttps: %s obtain/renew failed", domain)
			failed = true
		} else {
			logger.Logger.Info().Msgf("autohttps: %s certificate obtained", domain)
		}
	}
	return failed
}

// needsObtain 判断是否需要签发：无证书或进入续期窗口（含已过期——过期时
// now > notAfter 必然满足窗口条件，立即续）。
func (m *Manager) needsObtain(st *certState) bool {
	meta := st.leaf.Load()
	if meta == nil {
		return true
	}
	return needsRenewal(meta.notBefore, meta.notAfter, m.cfg.RenewalWindowRatio)
}

// adoptFromDisk 采纳磁盘上的证书变更（外部替换场景，certmagic reloadQueue 的单机版）：
// stat 未变直接跳过；内容变化且 serial 不同则热替换内存证书。
func (m *Manager) adoptFromDisk(st *certState) {
	fi, err := os.Stat(st.certFile)
	if err != nil {
		return // 无证书文件，无可采纳
	}
	if fi.ModTime().Equal(st.diskModTime) && fi.Size() == st.diskSize {
		return
	}
	cert, err := loadCertChain(st.certFile, st.keyFile)
	if err != nil {
		// 记录 stat 避免每轮重复解析；文件再次变化（外部修复）会重试
		st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
		logger.Logger.Error().Err(err).Msgf("autohttps: %s disk certificate unloadable", st.domain)
		return
	}
	cur := st.leaf.Load()
	if cur != nil && cur.serial.Cmp(cert.Leaf.SerialNumber) == 0 {
		st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
		return // 内容相同（如 mtime 被 touch），仅更新 stat
	}
	st.certPtr.Store(cert)
	st.leaf.Store(leafMetaFrom(cert.Leaf))
	st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
	logger.Logger.Info().Msgf("autohttps: %s adopted certificate from disk", st.domain)
}

// syncToDisk 补写未落盘的证书（签发成功但写盘失败时的收敛路径）。
func (m *Manager) syncToDisk(st *certState) {
	cur := st.certPtr.Load()
	if cur == nil || cur.Leaf == nil {
		return
	}
	disk, err := loadCertChain(st.certFile, st.keyFile)
	if err == nil && disk.Leaf.SerialNumber.Cmp(cur.Leaf.SerialNumber) == 0 {
		return // 磁盘已是同一张证书
	}
	if err := writeChainPEM(st.certFile, cur.Certificate, 0o644); err != nil {
		logger.Logger.Error().Err(err).Msgf("autohttps: %s sync cert to disk failed", st.domain)
		return
	}
	signer, ok := cur.PrivateKey.(crypto.Signer)
	if !ok {
		logger.Logger.Error().Msgf("autohttps: %s private key is not a signer, skip syncing key", st.domain)
		return
	}
	if err := writeKeyPEM(st.keyFile, signer, 0o600); err != nil {
		logger.Logger.Error().Err(err).Msgf("autohttps: %s sync key to disk failed", st.domain)
		return
	}
	if fi, err := os.Stat(st.certFile); err == nil {
		st.diskModTime, st.diskSize = fi.ModTime(), fi.Size()
	}
	logger.Logger.Info().Msgf("autohttps: %s synced certificate to disk", st.domain)
}

// setPending/clearPending 维护 pending 单槽，短临界区。
func (m *Manager) setPending(p *pendingChallenge) {
	m.mu.Lock()
	m.pending = p
	m.mu.Unlock()
}

func (m *Manager) clearPending() {
	m.mu.Lock()
	m.pending = nil
	m.mu.Unlock()
}

// needsRenewal 判断证书是否进入续期窗口：剩余寿命 ≤ 寿命 × ratio。
// notBefore/notAfter 零值或倒挂视为异常，返回 true（尽快重签）。
// 纯时间区间运算，时钟回拨免疫。
func needsRenewal(notBefore, notAfter time.Time, ratio float64) bool {
	if notBefore.IsZero() || notAfter.IsZero() || !notAfter.After(notBefore) {
		return true
	}
	lifetime := notAfter.Sub(notBefore)
	window := time.Duration(float64(lifetime) * ratio)
	return time.Now().After(notAfter.Add(-window))
}

// retryDelay 计算下一轮延迟：成功（failures==0）→ max（常规 CheckInterval）；
// 失败 → 指数退避 base×2^(n-1)，封顶 max。
func retryDelay(failures int, base, max time.Duration) time.Duration {
	if failures <= 0 {
		return max
	}
	d := base
	for i := 1; i < failures && d < max; i++ {
		d *= 2
	}
	if d > max {
		return max
	}
	return d
}

// leafMetaFrom 从叶子证书提取续期判定信息。
func leafMetaFrom(leaf *x509.Certificate) *leafMeta {
	if leaf == nil {
		return nil
	}
	return &leafMeta{
		serial:    leaf.SerialNumber,
		notBefore: leaf.NotBefore,
		notAfter:  leaf.NotAfter,
	}
}
