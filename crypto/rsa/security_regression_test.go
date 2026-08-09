package rsa

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"strconv"
	"testing"
)

func TestParseKeysRejectsWeakModulus(t *testing.T) {
	weakKey, err := stdrsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate weak test key: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(weakKey),
	})
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&weakKey.PublicKey),
	})

	if _, err := ParsePrivateKey(string(privatePEM)); !errors.Is(err, ErrInvalidKeySize) {
		t.Fatalf("ParsePrivateKey error = %v, want ErrInvalidKeySize", err)
	}
	if _, err := ParsePublicKey(string(publicPEM)); !errors.Is(err, ErrInvalidKeySize) || !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("ParsePublicKey error = %v, want both public-key and key-size errors", err)
	}
}

func TestParseKeysRejectsTrailingPEMData(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if _, err := ParsePrivateKey(mustPrivateKeyPEM(t, keyPair) + "unexpected"); !errors.Is(err, ErrInvalidPEMBlock) {
		t.Fatalf("ParsePrivateKey error = %v, want ErrInvalidPEMBlock", err)
	}
	if _, err := ParsePublicKey(mustPublicKeyPEM(t, keyPair) + "unexpected"); !errors.Is(err, ErrInvalidPEMBlock) {
		t.Fatalf("ParsePublicKey error = %v, want ErrInvalidPEMBlock", err)
	}
}

func TestParseKeysRejectsMismatchedPEMTypes(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(keyPair.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: privateDER})
	if _, err := ParsePrivateKey(string(privatePEM)); !errors.Is(err, ErrInvalidPEMBlock) {
		t.Fatalf("ParsePrivateKey error = %v, want ErrInvalidPEMBlock", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: publicDER})
	if _, err := ParsePublicKey(string(publicPEM)); !errors.Is(err, ErrInvalidPEMBlock) {
		t.Fatalf("ParsePublicKey error = %v, want ErrInvalidPEMBlock", err)
	}
}

func TestRSAOperationsRejectNilKeysWithoutPanicking(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "encrypt", run: func() error { _, err := EncryptOAEP([]byte("message"), nil); return err }, want: ErrInvalidPublicKey},
		{name: "decrypt", run: func() error { _, err := DecryptOAEP([]byte("ciphertext"), nil); return err }, want: ErrInvalidPrivateKey},
		{name: "sign", run: func() error { _, err := SignPSS([]byte("message"), nil); return err }, want: ErrInvalidPrivateKey},
		{name: "verify", run: func() error { return VerifyPSS([]byte("message"), []byte("signature"), nil) }, want: ErrInvalidPublicKey},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("operation panicked: %v", recovered)
				}
			}()
			if err := test.run(); !errors.Is(err, test.want) {
				t.Fatalf("operation error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestEncryptOAEPClassifiesOversizedMessage(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	maximum := keyPair.PublicKey.Size() - 2*sha256.Size - 2

	_, err = EncryptOAEP(make([]byte, maximum+1), keyPair.PublicKey)
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("EncryptOAEP error = %v, want ErrMessageTooLong", err)
	}
}

func TestSignPSSUsesSHA256SizedSalt(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	message := []byte("pss parameters")
	signature, err := SignPSS(message, keyPair.PrivateKey)
	if err != nil {
		t.Fatalf("SignPSS failed: %v", err)
	}
	digest := sha256.Sum256(message)
	options := &stdrsa.PSSOptions{SaltLength: stdrsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}

	if err := stdrsa.VerifyPSS(keyPair.PublicKey, crypto.SHA256, digest[:], signature, options); err != nil {
		t.Fatalf("signature does not use the explicit SHA-256 salt policy: %v", err)
	}
}

func TestVerifyPSSClassifiesInvalidSignature(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	err = VerifyPSS([]byte("message"), make([]byte, keyPair.PublicKey.Size()), keyPair.PublicKey)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("VerifyPSS error = %v, want ErrInvalidSignature", err)
	}
}

func TestKeyPairSerializationRejectsMissingKeys(t *testing.T) {
	keyPair := &KeyPair{}

	if _, err := keyPair.PrivateKeyToPEM(); !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatalf("PrivateKeyToPEM error = %v, want ErrInvalidPrivateKey", err)
	}
	if _, err := keyPair.PublicKeyToPEM(); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("PublicKeyToPEM error = %v, want ErrInvalidPublicKey", err)
	}
}

func TestNilKeyPairMethodsReturnErrors(t *testing.T) {
	var keyPair *KeyPair
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "encrypt", run: func() error { _, err := keyPair.Encrypt([]byte("message")); return err }, want: ErrInvalidPublicKey},
		{name: "decrypt", run: func() error { _, err := keyPair.Decrypt([]byte("ciphertext")); return err }, want: ErrInvalidPrivateKey},
		{name: "sign", run: func() error { _, err := keyPair.Sign([]byte("message")); return err }, want: ErrInvalidPrivateKey},
		{name: "verify", run: func() error { return keyPair.Verify([]byte("message"), []byte("signature")) }, want: ErrInvalidPublicKey},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("method panicked: %v", recovered)
				}
			}()
			if err := test.run(); !errors.Is(err, test.want) {
				t.Fatalf("method error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDecryptOAEPPreservesVagueStandardError(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	_, err = DecryptOAEP(make([]byte, keyPair.PublicKey.Size()), keyPair.PrivateKey)
	if !errors.Is(err, ErrDecryptionFailed) || !errors.Is(err, stdrsa.ErrDecryption) {
		t.Fatalf("DecryptOAEP error = %v, want both decryption errors", err)
	}
}

func TestPublicKeyValidationRejectsMalformedParameters(t *testing.T) {
	keyPair, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	evenModulus := &stdrsa.PublicKey{
		N: new(big.Int).Sub(keyPair.PublicKey.N, big.NewInt(1)),
		E: keyPair.PublicKey.E,
	}
	if err := validatePublicKey(evenModulus); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("even modulus validation error = %v, want ErrInvalidPublicKey", err)
	}

	if strconv.IntSize == 64 {
		oversizedExponent := &stdrsa.PublicKey{
			N: new(big.Int).Set(keyPair.PublicKey.N),
			E: int((int64(1) << 31) + 1),
		}
		if err := validatePublicKey(oversizedExponent); !errors.Is(err, ErrInvalidPublicKey) {
			t.Fatalf("oversized exponent validation error = %v, want ErrInvalidPublicKey", err)
		}
	}
}
