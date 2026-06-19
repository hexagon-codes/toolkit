// Package ssrf 提供 URL 级别的 SSRF（Server-Side Request Forgery）防护。
//
// 它在"发起请求前"对目标 URL 做校验：解析主机、做 DNS 解析并检查所有解析到的
// IP 是否落在私有/保留段，从而抵御 DNS rebinding；同时阻断 localhost 与云厂商
// 元数据端点（169.254.169.254 / metadata.google.internal）。
//
// 与 toolkit/net/httpx 的 WithSSRFProtection（连接期在 dial 时检查 IP）互补：
//   - httpx 是传输层防线（任何请求都拦私网 IP）；
//   - 本包是 URL 入口的前置校验（适合 FetchURL / 浏览器抓取 / MCP HTTP 等需要在
//     接受用户 URL 时就明确拒绝并给出原因的场景）。
//
// 对标 OpenClaw 的 fetchWithSsrfGuard。
package ssrf

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// privateRanges 私有/保留 IP 段。
//
// 比 Go 的 net.IP.IsPrivate 更全：除 RFC1918 外，额外覆盖 link-local（含云元数据
// 网段 169.254.0.0/16）、loopback、IPv6 唯一本地/link-local——这些都必须拦截。
var privateRanges = []net.IPNet{
	{IP: net.IP{10, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
	{IP: net.IP{172, 16, 0, 0}, Mask: net.CIDRMask(12, 32)},  // 172.16.0.0/12
	{IP: net.IP{192, 168, 0, 0}, Mask: net.CIDRMask(16, 32)}, // 192.168.0.0/16
	{IP: net.IP{169, 254, 0, 0}, Mask: net.CIDRMask(16, 32)}, // 169.254.0.0/16 (link-local)
	{IP: net.IP{127, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},    // 127.0.0.0/8 (loopback)
	{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},   // IPv6 loopback
	{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},  // IPv6 unique local
	{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)}, // IPv6 link-local
}

// blockedHosts 直接阻止的主机名（云厂商元数据端点等）。
var blockedHosts = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true, // GCP metadata
	"169.254.169.254":          true, // AWS/Azure/GCP metadata endpoint
}

// ValidateLocalURL 只允许 loopback 主机（localhost / 127.0.0.0/8 / ::1）。
//
// 用于"按定义就是本地"的 Provider（如 Ollama）：其 base URL 必须是 loopback，
// 绝不能是任意内网地址（云元数据端点 / 局域网主机）——否则一个"本地"Provider
// 就变成了 SSRF 跳板。
func ValidateLocalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL missing host")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// 非字面量、非 localhost 的主机可能解析到任意地址——拒绝。
		return fmt.Errorf("local provider URL must be loopback, got host %q", host)
	}
	if ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("local provider URL must be loopback, got %q", host)
}

// ValidateURL 校验 URL 是否安全（非内网/私有 IP）。
//
// 用于接受用户/模型提供的 URL 再发起请求的场景（抓取、浏览器、MCP HTTP 等）。
// 通过 DNS 解析后逐一检查解析 IP，抵御 DNS rebinding。
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL missing host")
	}

	// 检查直接阻止的主机名
	if blockedHosts[strings.ToLower(host)] {
		return fmt.Errorf("SSRF blocked: host %q is not allowed", host)
	}

	// 解析 IP
	ips, err := net.LookupHost(host)
	if err != nil {
		// DNS 解析失败时拒绝请求，防止 DNS rebinding 攻击
		return fmt.Errorf("DNS lookup failed for %s: %w", host, err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("SSRF blocked: %q resolves to unspecified address %s", host, ipStr)
		}
		for _, cidr := range privateRanges {
			if cidr.Contains(ip) {
				return fmt.Errorf("SSRF blocked: %q resolves to private IP %s", host, ipStr)
			}
		}
	}

	return nil
}
