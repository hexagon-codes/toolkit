package aes

import (
	"bytes"
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
	if err != ErrInvalidKeySize {
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
	key := "0123456789abcdef0123456789abcdef" // 32 bytes
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
	if err != ErrInvalidKeySize {
		t.Error("expected ErrInvalidKeySize")
	}

	_, err = DecryptGCM(plaintext, key)
	if err != ErrInvalidKeySize {
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

func TestEncryptDecryptCBC(t *testing.T) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! 你好世界")

	ciphertext, err := EncryptCBC(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptCBC failed: %v", err)
	}

	decrypted, err := DecryptCBC(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptCBC failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted text doesn't match original")
	}
}

func TestEncryptDecryptCBCString(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef"
	plaintext := "Hello, World!"

	ciphertext, err := EncryptCBCString(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptCBCString failed: %v", err)
	}

	decrypted, err := DecryptCBCString(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptCBCString failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted text doesn't match original")
	}
}

func TestCBCInvalidKey(t *testing.T) {
	key := []byte("short")
	plaintext := []byte("test")

	_, err := EncryptCBC(plaintext, key)
	if err != ErrInvalidKeySize {
		t.Error("expected ErrInvalidKeySize")
	}
}

func TestCBCInvalidCiphertext(t *testing.T) {
	key, _ := GenerateKey(32)

	// Too short
	_, err := DecryptCBC([]byte("short"), key)
	if err != ErrInvalidCiphertext {
		t.Error("expected ErrInvalidCiphertext")
	}

	// Wrong block size
	ciphertext := make([]byte, 20) // Not a multiple of block size after IV
	_, err = DecryptCBC(ciphertext, key)
	if err != ErrInvalidBlockSize {
		t.Error("expected ErrInvalidBlockSize")
	}
}

func TestEncryptDecryptCTR(t *testing.T) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! 你好世界")

	ciphertext, err := EncryptCTR(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptCTR failed: %v", err)
	}

	decrypted, err := DecryptCTR(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptCTR failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted text doesn't match original")
	}
}

func TestCTRInvalidKey(t *testing.T) {
	key := []byte("short")
	plaintext := []byte("test")

	_, err := EncryptCTR(plaintext, key)
	if err != ErrInvalidKeySize {
		t.Error("expected ErrInvalidKeySize")
	}
}

func TestCTRInvalidCiphertext(t *testing.T) {
	key, _ := GenerateKey(32)

	_, err := DecryptCTR([]byte("short"), key)
	if err != ErrInvalidCiphertext {
		t.Error("expected ErrInvalidCiphertext")
	}
}

func TestPKCS7Padding(t *testing.T) {
	data := []byte("test")
	padded := pkcs7Pad(data, 16)

	if len(padded)%16 != 0 {
		t.Error("padded data should be multiple of block size")
	}

	unpadded, err := pkcs7Unpad(padded)
	if err != nil {
		t.Fatalf("pkcs7Unpad failed: %v", err)
	}

	if !bytes.Equal(data, unpadded) {
		t.Error("unpadded data doesn't match original")
	}
}

func TestPKCS7InvalidPadding(t *testing.T) {
	// Empty data
	_, err := pkcs7Unpad([]byte{})
	if err != ErrInvalidPadding {
		t.Error("expected ErrInvalidPadding for empty data")
	}

	// Invalid padding value
	_, err = pkcs7Unpad([]byte{1, 2, 3, 0})
	if err != ErrInvalidPadding {
		t.Error("expected ErrInvalidPadding for zero padding")
	}

	// Padding larger than data
	_, err = pkcs7Unpad([]byte{1, 2, 3, 10})
	if err != ErrInvalidPadding {
		t.Error("expected ErrInvalidPadding for padding larger than data")
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

		// CBC
		ciphertext, err = EncryptCBC(plaintext, key)
		if err != nil {
			t.Errorf("EncryptCBC failed for key size %d: %v", size, err)
		}
		decrypted, err = DecryptCBC(ciphertext, key)
		if err != nil {
			t.Errorf("DecryptCBC failed for key size %d: %v", size, err)
		}
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("CBC decryption failed for key size %d", size)
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

func BenchmarkEncryptCBC(b *testing.B) {
	key, _ := GenerateKey(32)
	plaintext := []byte("Hello, World! This is a benchmark test message.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncryptCBC(plaintext, key)
	}
}
