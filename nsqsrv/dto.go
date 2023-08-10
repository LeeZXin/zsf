package nsqsrv

import "time"

type AuthRequestVO struct {
	RemoteIP   string `json:"remote_ip" form:"remote_ip"`
	TLS        bool   `json:"tls" form:"tls"`
	Secret     string `json:"secret" form:"secret"`
	CommonName string `json:"common_name" form:"common_name"`
}

type Authorization struct {
	Topic       string   `json:"topic"`       // 内容需要是符合正则表达式的形势，别问为啥，官方定的
	Channels    []string `json:"channels"`    // 内容需要是符合正则表达式的形势
	Permissions []string `json:"permissions"` // 订阅或者发布 subscribe ｜ publish

}

type AuthResponseVO struct {
	TTL            int             `json:"ttl"`            // 过期时间
	Authorizations []Authorization `json:"authorizations"` // 允许（或者不允许）哪些主体和通道
	Identity       string          `json:"identity"`       // 身份
	IdentityURL    string          `json:"identity_url"`   // 身份地址，就是提供验证的接口地址，例如这里本地的就是就是 127.0.0.1:1325
	Expires        time.Time
}
