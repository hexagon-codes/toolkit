package ssrf

import (
	"fmt"
	"strings"
	"testing"
)

// deterministicDNSFixtures keeps URL policy tests independent from the host
// resolver (including proxy fake-IP DNS). Public entries use documentation-only
// address ranges; private entries exercise every blocked address family.
var deterministicDNSFixtures = map[string][]string{
	"example.com":      {"192.0.2.10"},
	"google.com":       {"2001:db8::10"},
	"api.github.com":   {"198.51.100.10", "2001:db8::20"},
	"private-v4.test":  {"10.23.4.5"},
	"private-v6.test":  {"fe80::10"},
	"private-ula.test": {"fdfe:dcba:9876::98"},
	"loopback-v4.test": {"127.0.0.1"},
	"loopback-v6.test": {"::1"},
	"127.0.0.1":        {"127.0.0.1"},
	"::1":              {"::1"},
	"10.0.0.1":         {"10.0.0.1"},
	"172.16.0.1":       {"172.16.0.1"},
	"192.168.1.1":      {"192.168.1.1"},
	"169.254.169.254":  {"169.254.169.254"},
}

func deterministicLookupHost(host string) ([]string, error) {
	ips, ok := deterministicDNSFixtures[strings.ToLower(host)]
	if !ok {
		return nil, fmt.Errorf("no deterministic DNS fixture for %q", host)
	}
	return append([]string(nil), ips...), nil
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{"public URL", "https://api.github.com/repos", false, ""},
		{"localhost blocked", "http://localhost:8080/api", true, "SSRF blocked"},
		{"127.0.0.1 blocked", "http://127.0.0.1/secret", true, "SSRF blocked"},
		{"10.x blocked", "http://10.0.0.1/internal", true, "SSRF blocked"},
		{"172.16.x blocked", "http://172.16.0.1/admin", true, "SSRF blocked"},
		{"192.168.x blocked", "http://192.168.1.1/config", true, "SSRF blocked"},
		{"169.254 blocked", "http://169.254.169.254/latest/meta-data", true, "SSRF blocked"},
		{"resolved private IPv4 blocked", "http://private-v4.test/internal", true, "SSRF blocked"},
		{"resolved private IPv6 blocked", "http://private-v6.test/internal", true, "SSRF blocked"},
		{"resolved ULA blocked", "http://private-ula.test/internal", true, "SSRF blocked"},
		{"resolved IPv4 loopback blocked", "http://loopback-v4.test/internal", true, "SSRF blocked"},
		{"resolved IPv6 loopback blocked", "http://loopback-v6.test/internal", true, "SSRF blocked"},
		{"AWS metadata blocked", "http://169.254.169.254/", true, "SSRF blocked"},
		{"GCP metadata blocked", "http://metadata.google.internal/", true, "SSRF blocked"},
		{"empty URL", "", true, "missing host"},
		{"no host", "http://", true, "missing host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURLWithResolver(tt.url, deterministicLookupHost)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.url, err)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// The public production entry point must keep the real resolver path and must
// not special-case proxy fake-IP ULA space as public.
func TestValidateURLProductionDefaultResolverRejectsFakeIPULA(t *testing.T) {
	const fakeIPULA = "fdfe:dcba:9876::98"
	err := ValidateURL("http://[" + fakeIPULA + "]/")
	if err == nil {
		t.Fatal("expected production resolver path to block fake-IP ULA")
	}
	if !strings.Contains(err.Error(), "private IP "+fakeIPULA) {
		t.Fatalf("unexpected fake-IP ULA error: %v", err)
	}
}
