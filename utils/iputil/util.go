// Package iputil 提供本机 IPv4 地址获取工具。
package iputil

import (
	"net"
)

// GetIPV4 返回本机第一个非回环 IPv4 地址，无可用地址时返回空字符串。
// 注意：多网卡时返回的具体地址不保证（取决于系统枚举顺序）。
func GetIPV4() string {
	ips := AllIPV4()
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

// AllIPV4 返回本机所有非回环 IPv4 地址（不含 127.0.0.1）。
// 系统接口查询失败时返回 nil。
func AllIPV4() []string {
	adders, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	ipv4s := make([]string, 0)
	for _, addr := range adders {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ipv4 := ipNet.IP.String()
				if ipv4 == "127.0.0.1" || ipv4 == "localhost" {
					continue
				}
				ipv4s = append(ipv4s, ipv4)
			}
		}
	}
	return ipv4s
}
