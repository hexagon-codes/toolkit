package aes

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	tests := []int{16, 24, 32}

	for _, size := range tests {
		key, err := GenerateKey(size)
		if err != nil {
			t.Errorf("GenerateKey(%d) failed: %v", size, err)
		}
		if len(key) != size {
			t.Errorf("expected key length %d, got %d", size, len(key))
		}
	}

	// Invalid size
	_, err := GenerateKey(15)
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Error("expected ErrInvalidKeySize for invalid size")
	}
}

func TestGenerateKeyHex(t *testing.T) {
	hex, err := GenerateKeyHex(16)
	if err != nil {
		t.Fatalf("GenerateKeyHex failed: %v", err)
	}
	// Hex encoding doubles the length
	if len(hex) != 32 {
		t.Errorf("expected hex length 32, got %d", len(hex))
	}
}

func TestGenerateKeyBase64(t *testing.T) {
	b64, err := GenerateKeyBase64(16)
	if err != nil {
		t.Fatalf("GenerateKeyBase64 failed: %v", err)
	}
	if len(b64) == 0 {
		t.Error("expected non-empty base64 string")
	}
}

func TestEncryptDecryptGCM(t *testing.T) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! 你好世界")

	ciphertext, err := EncryptGCM(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptGCM failed: %v", err)
	}

	decrypted, err := DecryptGCM(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptGCM failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted text doesn't match original")
	}
}

func TestEncryptDecryptGCMString(t *testing.T) {
	key := strings.Repeat("a", 32)
	plaintext := "Hello, World!"

	ciphertext, err := EncryptGCMString(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptGCMString failed: %v", err)
	}

	decrypted, err := DecryptGCMString(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptGCMString failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted text doesn't match original")
	}
}

func TestGCMInvalidKey(t *testing.T) {
	key := []byte("short") // Invalid key size
	plaintext := []byte("test")

	_, err := EncryptGCM(plaintext, key)
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Error("expected ErrInvalidKeySize")
	}

	_, err = DecryptGCM(plaintext, key)
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Error("expected ErrInvalidKeySize")
	}
}

func TestGCMInvalidCiphertext(t *testing.T) {
	key, _ := GenerateKey(32)

	// Too short
	_, err := DecryptGCM([]byte("short"), key)
	if err != ErrInvalidCiphertext {
		t.Error("expected ErrInvalidCiphertext for short ciphertext")
	}
}

func TestDifferentKeySizes(t *testing.T) {
	plaintext := []byte("Hello, World!")

	for _, size := range []int{16, 24, 32} {
		key, _ := GenerateKey(size)

		// GCM
		ciphertext, err := EncryptGCM(plaintext, key)
		if err != nil {
			t.Errorf("EncryptGCM failed for key size %d: %v", size, err)
		}
		decrypted, err := DecryptGCM(ciphertext, key)
		if err != nil {
			t.Errorf("DecryptGCM failed for key size %d: %v", size, err)
		}
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("GCM decryption failed for key size %d", size)
		}
	}
}

func TestEmptyPlaintext(t *testing.T) {
	key, _ := GenerateKey(32)
	plaintext := []byte{}

	// GCM should handle empty plaintext
	ciphertext, err := EncryptGCM(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptGCM failed for empty plaintext: %v", err)
	}

	decrypted, err := DecryptGCM(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptGCM failed for empty plaintext: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("decrypted empty plaintext doesn't match")
	}
}

func BenchmarkEncryptGCM(b *testing.B) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! This is a benchmark test message.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncryptGCM(plaintext, key)
	}
}

func BenchmarkDecryptGCM(b *testing.B) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! This is a benchmark test message.")
	ciphertext, _ := EncryptGCM(plaintext, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecryptGCM(ciphertext, key)
	}
}
