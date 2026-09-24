// Package autohttps 提供 ACME 证书的自动申请与续期：按配置管理一组域名的
// HTTPS 证书，到期前自动续期并热替换内存证书，无需重启服务。
//
// 设计参考 Caddy/certmagic 的维护循环思路（续期窗口判定 + 后台循环 + 内存
// 证书缓存），按单机场景精简：固定文件落盘（无插件化存储）、单维护 goroutine、
// 无 ARI、无 on-demand。
//
// 使用方式（按需接入，zsf 是通用框架，本包不默认装配任何服务）：
//
//	mgr, err := autohttps.NewManager(autohttps.Config{
//	    Certs: []autohttps.Cert{
//	        {Domain: "a.example.com"},
//	        {Domain: "b.example.com", CertFile: "acme/b.crt", KeyFile: "acme/b.key"},
//	    },
//	    Email: "ops@example.com",
//	})
//	if err != nil {
//	    logger.Logger.Fatal().Err(err).Msg("create autohttps manager")
//	}
//
//	// 443 服务：动态证书回调（证书热替换 + 按 SNI 分派）
//	httpsServer := server.NewDefaultServer(
//	    server.WithHttpPort(443),
//	    server.WithGetCertificate(mgr.GetCertificate),
//	)
//	// 80 服务：注册 HTTP-01 挑战中间件（必须排在 80→443 redirect 等拦截中间件之前）
//	httpServer := server.NewDefaultServer(
//	    server.WithHttpPort(80),
//	    server.AddFilters(mgr.HTTP01Handler()),
//	)
//
//	lifecycle.Run(lifecycle.WithObjects(mgr, httpsServer, httpServer))
//
// 挑战方式：优先 TLS-ALPN-01（CA 直连 443 端口验证，零配置），CA 不支持时
// 回退 HTTP-01（需把 HTTP01Handler 注册到 80 服务）。两者都不支持（如通配符
// 域名需 DNS-01）时签发失败并退避重试。
//
// 注意：
//   - 证书文件路径相对 resources 目录（绝对路径原样使用）；私钥 0600、证书 0644；
//   - ACME 账号私钥默认 resources/acme/account.key，一旦生成不要删除/更换，
//     否则等于更换 ACME 账号（Let's Encrypt 对同域名新账号有签发频控）；
//   - 同一域名不要配置多个 Manager，重复签发会触发 CA 频控；
//   - 磁盘证书被外部程序（certbot 等）替换时，下一检查周期自动采纳并用于握手。
package autohttps
