package rsa

import (
	"bytes"
	"errors"
	"testing"
)

func mustPrivateKeyPEM(t *testing.T, keyPair *KeyPair) string {
	t.Helper()
	encoded, err := keyPair.PrivateKeyToPEM()
	if err != nil {
		t.Fatalf("PrivateKeyToPEM failed: %v", err)
	}
	return encoded
}

func mustPublicKeyPEM(t *testing.T, keyPair *KeyPair) string {
	t.Helper()
	encoded, err := keyPair.PublicKeyToPEM()
	if err != nil {
		t.Fatalf("PublicKeyToPEM failed: %v", err)
	}
	return encoded
}

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if kp.PrivateKey == nil {
		t.Error("private key is nil")
	}
	if kp.PublicKey == nil {
		t.Error("public key is nil")
	}
}

func TestGenerateKeyPairInvalidSize(t *testing.T) {
	_, err := GenerateKeyPair(512)
	if err != ErrInvalidKeySize {
		t.Error("expected ErrInvalidKeySize for small key")
	}
}

func TestKeyPairToPEM(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	privatePEM := mustPrivateKeyPEM(t, kp)
	if privatePEM == "" {
		t.Error("private PEM is empty")
	}

	publicPEM := mustPublicKeyPEM(t, kp)
	if publicPEM == "" {
		t.Error("public PEM is empty")
	}

	// Verify they contain correct headers
	if len(privatePEM) < 50 {
		t.Error("private PEM seems too short")
	}
	if len(publicPEM) < 50 {
		t.Error("public PEM seems too short")
	}
}

func TestKeyPairToPKCS8PEM(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	privatePEM, err := kp.PrivateKeyToPKCS8PEM()
	if err != nil {
		t.Fatalf("PrivateKeyToPKCS8PEM failed: %v", err)
	}
	if privatePEM == "" {
		t.Error("PKCS8 private PEM is empty")
	}

	publicPEM, err := kp.PublicKeyToPKIXPEM()
	if err != nil {
		t.Fatalf("PublicKeyToPKIXPEM failed: %v", err)
	}
	if publicPEM == "" {
		t.Error("PKIX public PEM is empty")
	}
}

func TestParsePrivateKey(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	// PKCS1 format
	pem := mustPrivateKeyPEM(t, kp)
	parsed, err := ParsePrivateKey(pem)
	if err != nil {
		t.Fatalf("ParsePrivateKey (PKCS1) failed: %v", err)
	}
	if parsed == nil {
		t.Error("parsed private key is nil")
	}

	// PKCS8 format
	pkcs8PEM, _ := kp.PrivateKeyToPKCS8PEM()
	parsed, err = ParsePrivateKey(pkcs8PEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey (PKCS8) failed: %v", err)
	}
	if parsed == nil {
		t.Error("parsed private key is nil")
	}
}

func TestParsePublicKey(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	// PKCS1 format
	pem := mustPublicKeyPEM(t, kp)
	parsed, err := ParsePublicKey(pem)
	if err != nil {
		t.Fatalf("ParsePublicKey (PKCS1) failed: %v", err)
	}
	if parsed == nil {
		t.Error("parsed public key is nil")
	}

	// PKIX format
	pkixPEM, _ := kp.PublicKeyToPKIXPEM()
	parsed, err = ParsePublicKey(pkixPEM)
	if err != nil {
		t.Fatalf("ParsePublicKey (PKIX) failed: %v", err)
	}
	if parsed == nil {
		t.Error("parsed public key is nil")
	}
}

func TestParseInvalidPEM(t *testing.T) {
	_, err := ParsePrivateKey("not a pem")
	if err != ErrInvalidPEMBlock {
		t.Error("expected ErrInvalidPEMBlock for invalid PEM")
	}

	_, err = ParsePublicKey("not a pem")
	if err != ErrInvalidPEMBlock {
		t.Error("expected ErrInvalidPEMBlock for invalid PEM")
	}
}

func TestEncryptDecryptOAEP(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	plaintext := []byte("Hello, World!")

	ciphertext, err := EncryptOAEP(plaintext, kp.PublicKey)
	if err != nil {
		t.Fatalf("EncryptOAEP failed: %v", err)
	}

	decrypted, err := DecryptOAEP(ciphertext, kp.PrivateKey)
	if err != nil {
		t.Fatalf("DecryptOAEP failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("decrypted text doesn't match original")
	}
}

func TestEncryptDecryptOAEPString(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	plaintext := "Hello, World!"

	publicPEM := mustPublicKeyPEM(t, kp)
	privatePEM := mustPrivateKeyPEM(t, kp)

	ciphertext, err := EncryptOAEPString(plaintext, publicPEM)
	if err != nil {
		t.Fatalf("EncryptOAEPString failed: %v", err)
	}

	decrypted, err := DecryptOAEPString(ciphertext, privatePEM)
	if err != nil {
		t.Fatalf("DecryptOAEPString failed: %v", err)
	}

	if decrypted != plaintext {
		t.Error("decrypted text doesn't match original")
	}
}

func TestSignVerifyPSS(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	message := []byte("Hello, World!")

	signature, err := SignPSS(message, kp.PrivateKey)
	if err != nil {
		t.Fatalf("SignPSS failed: %v", err)
	}

	err = VerifyPSS(message, signature, kp.PublicKey)
	if err != nil {
		t.Fatalf("VerifyPSS failed: %v", err)
	}
}

func TestSignVerifyPSS_InvalidSignature(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	message := []byte("Hello, World!")

	signature, _ := SignPSS(message, kp.PrivateKey)

	// Tamper with signature
	signature[0] ^= 0xFF

	err := VerifyPSS(message, signature, kp.PublicKey)
	if err == nil {
		t.Error("expected error for tampered signature")
	}
}

func TestSignVerifyString(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	message := "Hello, World!"

	privatePEM := mustPrivateKeyPEM(t, kp)
	publicPEM := mustPublicKeyPEM(t, kp)

	signature, err := SignString(message, privatePEM)
	if err != nil {
		t.Fatalf("SignString failed: %v", err)
	}

	err = VerifyString(message, signature, publicPEM)
	if err != nil {
		t.Fatalf("VerifyString failed: %v", err)
	}
}

func TestSignVerifyString_TamperedMessage(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	privatePEM := mustPrivateKeyPEM(t, kp)
	publicPEM := mustPublicKeyPEM(t, kp)

	signature, _ := SignString("Hello, World!", privatePEM)

	err := VerifyString("Tampered Message", signature, publicPEM)
	if err == nil {
		t.Error("expected error for tampered message")
	}
}

func TestKeyPairMethods(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)
	plaintext := []byte("Hello, World!")

	// Encrypt/Decrypt
	ciphertext, err := kp.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("KeyPair.Encrypt failed: %v", err)
	}

	decrypted, err := kp.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("KeyPair.Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("decrypted text doesn't match")
	}

	// Sign/Verify
	message := []byte("Message to sign")
	signature, err := kp.Sign(message)
	if err != nil {
		t.Fatalf("KeyPair.Sign failed: %v", err)
	}

	err = kp.Verify(message, signature)
	if err != nil {
		t.Fatalf("KeyPair.Verify failed: %v", err)
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	kp, _ := GenerateKeyPair(2048)

	_, err := DecryptOAEP([]byte("invalid ciphertext"), kp.PrivateKey)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Error("expected ErrDecryptionFailed for invalid ciphertext")
	}
}

func BenchmarkGenerateKeyPair2048(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateKeyPair(2048)
	}
}

func BenchmarkEncryptOAEP(b *testing.B) {
	kp, _ := GenerateKeyPair(2048)
	plaintext := []byte("Hello, World! This is a benchmark test.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EncryptOAEP(plaintext, kp.PublicKey)
	}
}

func BenchmarkDecryptOAEP(b *testing.B) {
	kp, _ := GenerateKeyPair(2048)
	plaintext := []byte("Hello, World! This is a benchmark test.")
	ciphertext, _ := EncryptOAEP(plaintext, kp.PublicKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecryptOAEP(ciphertext, kp.PrivateKey)
	}
}

func BenchmarkSignPSS(b *testing.B) {
	kp, _ := GenerateKeyPair(2048)
	message := []byte("Hello, World! This is a benchmark test.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SignPSS(message, kp.PrivateKey)
	}
}

func BenchmarkVerifyPSS(b *testing.B) {
	kp, _ := GenerateKeyPair(2048)
	message := []byte("Hello, World! This is a benchmark test.")
	signature, _ := SignPSS(message, kp.PrivateKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyPSS(message, signature, kp.PublicKey)
	}
}
