package ip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		ip    string
		valid bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"255.255.255.255", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"invalid", false},
		{"256.1.1.1", false},
		{"", false},
	}

	for _, tt := range tests {
		if IsValid(tt.ip) != tt.valid {
			t.Errorf("IsValid(%s) = %v, want %v", tt.ip, IsValid(tt.ip), tt.valid)
		}
	}
}

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		ip     string
		isIPv4 bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"::1", false},
		{"2001:db8::1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if IsIPv4(tt.ip) != tt.isIPv4 {
			t.Errorf("IsIPv4(%s) = %v, want %v", tt.ip, IsIPv4(tt.ip), tt.isIPv4)
		}
	}
}

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		ip     string
		isIPv6 bool
	}{
		{"::1", true},
		{"2001:db8::1", true},
		{"fe80::1", true},
		{"192.168.1.1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if IsIPv6(tt.ip) != tt.isIPv6 {
			t.Errorf("IsIPv6(%s) = %v, want %v", tt.ip, IsIPv6(tt.ip), tt.isIPv6)
		}
	}
}

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		ip        string
		isPrivate bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if IsPrivate(tt.ip) != tt.isPrivate {
			t.Errorf("IsPrivate(%s) = %v, want %v", tt.ip, IsPrivate(tt.ip), tt.isPrivate)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		ip         string
		isLoopback bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		{"192.168.1.1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if IsLoopback(tt.ip) != tt.isLoopback {
			t.Errorf("IsLoopback(%s) = %v, want %v", tt.ip, IsLoopback(tt.ip), tt.isLoopback)
		}
	}
}

func TestIsPublic(t *testing.T) {
	tests := []struct {
		ip       string
		isPublic bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"192.168.1.1", false}, // private
		{"127.0.0.1", false},   // loopback
		{"0.0.0.0", false},     // unspecified
		{"invalid", false},
	}

	for _, tt := range tests {
		if IsPublic(tt.ip) != tt.isPublic {
			t.Errorf("IsPublic(%s) = %v, want %v", tt.ip, IsPublic(tt.ip), tt.isPublic)
		}
	}
}

func TestIsInCIDR(t *testing.T) {
	tests := []struct {
		ip     string
		cidr   string
		result bool
	}{
		{"192.168.1.100", "192.168.1.0/24", true},
		{"192.168.2.1", "192.168.1.0/24", false},
		{"10.0.0.50", "10.0.0.0/8", true},
		{"invalid", "192.168.1.0/24", false},
		{"192.168.1.1", "invalid", false},
	}

	for _, tt := range tests {
		if IsInCIDR(tt.ip, tt.cidr) != tt.result {
			t.Errorf("IsInCIDR(%s, %s) = %v, want %v", tt.ip, tt.cidr, IsInCIDR(tt.ip, tt.cidr), tt.result)
		}
	}
}

func TestIsInRange(t *testing.T) {
	tests := []struct {
		ip     string
		start  string
		end    string
		result bool
	}{
		{"192.168.1.50", "192.168.1.1", "192.168.1.100", true},
		{"192.168.1.1", "192.168.1.1", "192.168.1.100", true},   // boundary
		{"192.168.1.100", "192.168.1.1", "192.168.1.100", true}, // boundary
		{"192.168.1.101", "192.168.1.1", "192.168.1.100", false},
		{"192.168.0.1", "192.168.1.1", "192.168.1.100", false},
		{"invalid", "192.168.1.1", "192.168.1.100", false},
	}

	for _, tt := range tests {
		if IsInRange(tt.ip, tt.start, tt.end) != tt.result {
			t.Errorf("IsInRange(%s, %s, %s) = %v, want %v", tt.ip, tt.start, tt.end, IsInRange(tt.ip, tt.start, tt.end), tt.result)
		}
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := GetLocalIP()
	if err != nil {
		// This might fail in some test environments, so just log
		t.Logf("GetLocalIP error (may be expected): %v", err)
		return
	}

	if !IsValid(ip) {
		t.Errorf("GetLocalIP returned invalid IP: %s", ip)
	}
}

func TestGetLocalIPs(t *testing.T) {
	ips, err := GetLocalIPs()
	if err != nil {
		t.Logf("GetLocalIPs error (may be expected): %v", err)
		return
	}

	for _, ip := range ips {
		if !IsValid(ip) {
			t.Errorf("GetLocalIPs returned invalid IP: %s", ip)
		}
	}
}

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For single",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8, 9.10.11.12"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "1.2.3.4",
		},
		{
			name:       "CF-Connecting-IP",
			headers:    map[string]string{"CF-Connecting-IP": "1.2.3.4"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "1.2.3.4",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "Invalid header IP",
			headers:    map[string]string{"X-Real-IP": "invalid"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := FromRequest(req)
			if ip != tt.expected {
				t.Errorf("FromRequest() = %s, want %s", ip, tt.expected)
			}
		})
	}
}

func TestIPv4ToInt(t *testing.T) {
	tests := []struct {
		ip       string
		expected uint32
	}{
		{"0.0.0.0", 0},
		{"0.0.0.1", 1},
		{"0.0.1.0", 256},
		{"0.1.0.0", 65536},
		{"1.0.0.0", 16777216},
		{"192.168.1.1", 3232235777},
		{"255.255.255.255", 4294967295},
		{"invalid", 0},
		{"::1", 0}, // IPv6
	}

	for _, tt := range tests {
		result := IPv4ToInt(tt.ip)
		if result != tt.expected {
			t.Errorf("IPv4ToInt(%s) = %d, want %d", tt.ip, result, tt.expected)
		}
	}
}

func TestIntToIPv4(t *testing.T) {
	tests := []struct {
		n        uint32
		expected string
	}{
		{0, "0.0.0.0"},
		{1, "0.0.0.1"},
		{3232235777, "192.168.1.1"},
		{4294967295, "255.255.255.255"},
	}

	for _, tt := range tests {
		result := IntToIPv4(tt.n)
		if result != tt.expected {
			t.Errorf("IntToIPv4(%d) = %s, want %s", tt.n, result, tt.expected)
		}
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		ip       string
		mask     int
		expected string
	}{
		{"192.168.1.100", 24, "192.168.1.0"},
		{"192.168.1.100", 16, "192.168.0.0"},
		{"192.168.1.100", 8, "192.0.0.0"},
		{"invalid", 24, ""},
	}

	for _, tt := range tests {
		result := Mask(tt.ip, tt.mask)
		if result != tt.expected {
			t.Errorf("Mask(%s, %d) = %s, want %s", tt.ip, tt.mask, result, tt.expected)
		}
	}
}

func TestCIDRContains(t *testing.T) {
	// Same as IsInCIDR with different parameter order
	if !CIDRContains("192.168.1.0/24", "192.168.1.100") {
		t.Error("CIDRContains should return true")
	}

	if CIDRContains("192.168.1.0/24", "192.168.2.1") {
		t.Error("CIDRContains should return false")
	}
}

func TestParseCIDR(t *testing.T) {
	ip, network, err := ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Errorf("ParseCIDR error: %v", err)
	}

	if ip.String() != "192.168.1.0" {
		t.Errorf("expected 192.168.1.0, got %s", ip.String())
	}

	if network.String() != "192.168.1.0/24" {
		t.Errorf("expected 192.168.1.0/24, got %s", network.String())
	}

	// Invalid CIDR
	_, _, err = ParseCIDR("invalid")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestResolveHost(t *testing.T) {
	// This test depends on network, may fail in some environments
	ips, err := ResolveHost("localhost")
	if err != nil {
		t.Logf("ResolveHost error (may be expected): %v", err)
		return
	}

	if len(ips) == 0 {
		t.Error("ResolveHost should return at least one IP")
	}
}

func TestGetMACAddress(t *testing.T) {
	mac, err := GetMACAddress()
	if err != nil {
		t.Logf("GetMACAddress error (may be expected): %v", err)
		return
	}

	// MAC address format: XX:XX:XX:XX:XX:XX
	if mac != "" && len(mac) != 17 {
		t.Logf("Unexpected MAC address format: %s", mac)
	}
}
