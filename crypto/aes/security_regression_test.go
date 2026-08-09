package aes

import (
	standardaes "crypto/aes"
	"errors"
	"testing"
)

func TestDecryptGCMRejectsCiphertextWithoutAuthenticationTag(t *testing.T) {
	key := make([]byte, 32)
	ciphertext := make([]byte, 12)

	_, err := DecryptGCM(ciphertext, key)
	if !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("DecryptGCM error = %v, want ErrInvalidCiphertext", err)
	}
}

func TestDecryptGCMClassifiesAuthenticationFailure(t *testing.T) {
	key := make([]byte, 32)
	ciphertext, err := EncryptGCM([]byte("authenticated"), key)
	if err != nil {
		t.Fatalf("EncryptGCM failed: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = DecryptGCM(ciphertext, key)
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("DecryptGCM error = %v, want ErrAuthenticationFailed", err)
	}
}

func TestGCMInvalidKeyPreservesStandardError(t *testing.T) {
	operations := map[string]func() error{
		"encrypt": func() error {
			_, err := EncryptGCM([]byte("plaintext"), []byte("short"))
			return err
		},
		"decrypt": func() error {
			_, err := DecryptGCM([]byte("ciphertext"), []byte("short"))
			return err
		},
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if !errors.Is(err, ErrInvalidKeySize) {
				t.Fatalf("operation error = %v, want ErrInvalidKeySize", err)
			}
			var keySizeError standardaes.KeySizeError
			if !errors.As(err, &keySizeError) {
				t.Fatalf("operation error = %v, want crypto/aes.KeySizeError in chain", err)
			}
		})
	}
}
