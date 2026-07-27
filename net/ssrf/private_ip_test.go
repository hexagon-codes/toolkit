package ssrf

import (
	"testing"
)

// TestSSRF_PrivateIPs 覆盖公网、私网、loopback、ULA 与云元数据端点，
// 并通过确定性 resolver 避免宿主 fake-IP DNS 改写测试结论。
func TestSSRF_PrivateIPs(t *testing.T) {
	tests := []struct {
		url  string
		safe bool
	}{
		{"https://example.com/api", true},
		{"https://google.com", true},
		{"http://localhost:8080", false},
		{"http://127.0.0.1:3000", false},
		{"http://[::1]:8080", false},
		{"http://169.254.169.254/latest/meta-data", false},
		{"http://metadata.google.internal", false},
		{"http://private-v4.test", false},
		{"http://private-v6.test", false},
		{"http://private-ula.test", false},
		{"http://loopback-v4.test", false},
		{"http://loopback-v6.test", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateURLWithResolver(tt.url, deterministicLookupHost)
			if tt.safe && err != nil {
				t.Errorf("expected safe URL, got error: %v", err)
			}
			if !tt.safe && err == nil {
				t.Errorf("expected SSRF block for: %s", tt.url)
			}
		})
	}
}
