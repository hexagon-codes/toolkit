package hash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootReadmesDescribeSupportedHashAlgorithms(t *testing.T) {
	tests := map[string]string{
		"README.md":    "├── hash/          # 哈希（SHA-256/SHA-512/bcrypt）",
		"README.en.md": "├── hash/          # Hashing (SHA-256/SHA-512/bcrypt)",
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", name))
			if err != nil {
				t.Fatalf("read root README: %v", err)
			}
			if !strings.Contains(string(content), want) {
				t.Fatalf("root README does not describe the supported hash algorithms; want line %q", want)
			}
		})
	}
}
