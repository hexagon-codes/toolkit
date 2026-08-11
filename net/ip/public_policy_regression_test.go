package ip

import (
	"net"
	"testing"
)

func TestIsPrivateOrReservedIPRejectsSpecialUseAddresses(t *testing.T) {
	t.Parallel()

	addresses := []string{
		"100.64.0.1",      // 共享地址空间
		"192.0.0.1",       // IETF 协议分配
		"192.0.2.1",       // 文档地址
		"192.88.99.1",     // 已废弃的 6to4 中继
		"198.18.0.1",      // 基准测试地址
		"198.51.100.1",    // 文档地址
		"203.0.113.1",     // 文档地址
		"240.0.0.1",       // 保留地址
		"64:ff9b::7f00:1", // NAT64 可映射到 IPv4 回环地址
		"100::1",          // 丢弃前缀
		"2001:db8::1",     // IPv6 文档地址
		"2002:7f00:1::1",  // 6to4 可嵌入 IPv4 回环地址
		"3fff::1",         // IPv6 文档地址
		"5f00::1",         // IPv6 段路由保留前缀
		"fec0::1",         // 已废弃的站点本地地址
	}

	for _, address := range addresses {
		address := address
		t.Run(address, func(t *testing.T) {
			parsed := net.ParseIP(address)
			if parsed == nil {
				t.Fatalf("failed to parse test address %q", address)
			}
			if !IsPrivateOrReservedIP(parsed) {
				t.Fatalf("IsPrivateOrReservedIP(%q) = false, want true", address)
			}
			if IsPublic(address) {
				t.Fatalf("IsPublic(%q) = true, want false", address)
			}
		})
	}
}

func TestIsPrivateOrReservedIPUnmapsIPv4MappedIPv6(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"::ffff:127.0.0.1",
		"::ffff:169.254.169.254",
		"::ffff:192.168.1.1",
	} {
		parsed := net.ParseIP(address)
		if parsed == nil {
			t.Fatalf("failed to parse test address %q", address)
		}
		if !IsPrivateOrReservedIP(parsed) {
			t.Fatalf("IsPrivateOrReservedIP(%q) = false, want true", address)
		}
	}
}
