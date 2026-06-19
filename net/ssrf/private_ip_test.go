package ssrf

import (
	"testing"
)

// TestSSRF_PrivateIPs 覆盖私网/loopback/云元数据端点的拦截（自 hexclaw 下沉时一并迁入）。
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
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if tt.safe && err != nil {
				t.Errorf("expected safe URL, got error: %v", err)
			}
			if !tt.safe && err == nil {
				t.Errorf("expected SSRF block for: %s", tt.url)
			}
		})
	}
}
