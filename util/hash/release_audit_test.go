package hash

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestDigestOutputUsesCanonicalLowercaseHex(t *testing.T) {
	tests := []struct {
		name   string
		length int
		digest string
	}{
		{name: "sha256 string", length: 64, digest: SHA256("release-audit")},
		{name: "sha256 bytes", length: 64, digest: SHA256Bytes([]byte("release-audit"))},
		{name: "sha512 string", length: 128, digest: SHA512("release-audit")},
		{name: "sha512 bytes", length: 128, digest: SHA512Bytes([]byte("release-audit"))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.digest) != test.length {
				t.Fatalf("digest length = %d, want %d", len(test.digest), test.length)
			}
			if test.digest != strings.ToLower(test.digest) {
				t.Fatalf("digest is not lowercase: %q", test.digest)
			}
			if _, err := hex.DecodeString(test.digest); err != nil {
				t.Fatalf("digest is not canonical hexadecimal: %v", err)
			}
		})
	}
}
