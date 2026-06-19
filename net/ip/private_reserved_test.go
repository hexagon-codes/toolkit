package ip

import (
	"net"
	"testing"
)

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// 内网 / 保留 —— 必须拦截
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"rfc1918 10", "10.1.2.3", true},
		{"rfc1918 172", "172.16.0.1", true},
		{"rfc1918 192.168", "192.168.1.1", true},
		{"ula v6", "fc00::1", true},
		{"link-local", "169.254.1.1", true},
		{"cloud metadata", "169.254.169.254", true}, // AWS/GCP/Azure 元数据，SSRF 重点
		{"link-local v6", "fe80::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		// 公网 —— 不应拦截
		{"public v4", "8.8.8.8", false},
		{"public v4 2", "1.1.1.1", false},
		{"public v6", "2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("无法解析 IP %q", tt.ip)
			}
			if got := IsPrivateOrReservedIP(ip); got != tt.want {
				t.Errorf("IsPrivateOrReservedIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}

	// nil 入参返回 false
	if IsPrivateOrReservedIP(nil) {
		t.Error("IsPrivateOrReservedIP(nil) 应返回 false")
	}
}
