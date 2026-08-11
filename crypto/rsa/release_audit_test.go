package rsa

import (
	stdrsa "crypto/rsa"
	"errors"
	"testing"
)

func TestDecryptOAEPReturnsOneUniformPublicError(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	ciphertexts := [][]byte{
		nil,
		make([]byte, keyPair.PublicKey.Size()-1),
		make([]byte, keyPair.PublicKey.Size()),
		make([]byte, keyPair.PublicKey.Size()+1),
	}
	for _, ciphertext := range ciphertexts {
		_, decryptErr := DecryptOAEP(ciphertext, keyPair.PrivateKey)
		if decryptErr != ErrDecryptionFailed {
			t.Fatalf("DecryptOAEP(%d bytes) error = %v, want the uniform ErrDecryptionFailed sentinel", len(ciphertext), decryptErr)
		}
		if errors.Is(decryptErr, stdrsa.ErrDecryption) {
			t.Fatalf("DecryptOAEP(%d bytes) exposed the underlying RSA decryption error", len(ciphertext))
		}
	}
}
