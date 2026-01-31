package sign

import (
	"testing"
)

func TestHMACSHA256(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA256(message, key)
	if len(sig) != 32 { // SHA256 produces 32 bytes
		t.Errorf("expected 32 bytes, got %d", len(sig))
	}
}

func TestHMACSHA256Hex(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA256Hex(message, key)
	if len(sig) != 64 { // 32 bytes * 2 hex chars
		t.Errorf("expected 64 chars, got %d", len(sig))
	}
}

func TestHMACSHA256Base64(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA256Base64(message, key)
	if sig == "" {
		t.Error("expected non-empty base64 string")
	}
}

func TestHMACSHA256String(t *testing.T) {
	sig := HMACSHA256String("Hello, World!", "secret-key")
	if len(sig) != 64 {
		t.Errorf("expected 64 chars, got %d", len(sig))
	}
}

func TestHMACSHA512(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA512(message, key)
	if len(sig) != 64 { // SHA512 produces 64 bytes
		t.Errorf("expected 64 bytes, got %d", len(sig))
	}
}

func TestHMACSHA512Hex(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA512Hex(message, key)
	if len(sig) != 128 { // 64 bytes * 2 hex chars
		t.Errorf("expected 128 chars, got %d", len(sig))
	}
}

func TestHMACSHA512String(t *testing.T) {
	sig := HMACSHA512String("Hello, World!", "secret-key")
	if len(sig) != 128 {
		t.Errorf("expected 128 chars, got %d", len(sig))
	}
}

func TestVerifyHMACSHA256(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA256(message, key)

	if !VerifyHMACSHA256(message, key, sig) {
		t.Error("verification should pass")
	}

	// Tampered signature
	sig[0] ^= 0xFF
	if VerifyHMACSHA256(message, key, sig) {
		t.Error("verification should fail for tampered signature")
	}
}

func TestVerifyHMACSHA256Hex(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sigHex := HMACSHA256Hex(message, key)

	if !VerifyHMACSHA256Hex(message, key, sigHex) {
		t.Error("verification should pass")
	}

	// Invalid hex
	if VerifyHMACSHA256Hex(message, key, "invalid-hex") {
		t.Error("verification should fail for invalid hex")
	}
}

func TestVerifyHMACSHA256Base64(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sigBase64 := HMACSHA256Base64(message, key)

	if !VerifyHMACSHA256Base64(message, key, sigBase64) {
		t.Error("verification should pass")
	}

	// Invalid base64
	if VerifyHMACSHA256Base64(message, key, "!!!invalid!!!") {
		t.Error("verification should fail for invalid base64")
	}
}

func TestVerifyHMACSHA256String(t *testing.T) {
	message := "Hello, World!"
	key := "secret-key"

	sig := HMACSHA256String(message, key)

	if !VerifyHMACSHA256String(message, key, sig) {
		t.Error("verification should pass")
	}

	if VerifyHMACSHA256String("Tampered", key, sig) {
		t.Error("verification should fail for tampered message")
	}
}

func TestVerifyHMACSHA512(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMACSHA512(message, key)

	if !VerifyHMACSHA512(message, key, sig) {
		t.Error("verification should pass")
	}
}

func TestVerifyHMACSHA512Hex(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sigHex := HMACSHA512Hex(message, key)

	if !VerifyHMACSHA512Hex(message, key, sigHex) {
		t.Error("verification should pass")
	}
}

func TestVerifyHMACSHA512String(t *testing.T) {
	message := "Hello, World!"
	key := "secret-key"

	sig := HMACSHA512String(message, key)

	if !VerifyHMACSHA512String(message, key, sig) {
		t.Error("verification should pass")
	}
}

func TestHMAC(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	tests := []struct {
		hashType HMACHash
		length   int
	}{
		{SHA256, 32},
		{SHA512, 64},
		{SHA384, 48},
		{SHA224, 28},
	}

	for _, tt := range tests {
		sig := HMAC(message, key, tt.hashType)
		if len(sig) != tt.length {
			t.Errorf("HMAC(%v): expected %d bytes, got %d", tt.hashType, tt.length, len(sig))
		}
	}
}

func TestHMACHex(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	hex := HMACHex(message, key, SHA256)
	if len(hex) != 64 {
		t.Errorf("expected 64 chars, got %d", len(hex))
	}
}

func TestVerifyHMAC(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig := HMAC(message, key, SHA256)

	if !VerifyHMAC(message, key, sig, SHA256) {
		t.Error("verification should pass")
	}

	if VerifyHMAC([]byte("Tampered"), key, sig, SHA256) {
		t.Error("verification should fail for tampered message")
	}
}

func TestTimestampSigner(t *testing.T) {
	signer := NewTimestampSigner([]byte("secret-key"))

	message := "Hello, World!"
	timestamp := int64(1704067200)

	sig := signer.Sign(message, timestamp)
	if sig == "" {
		t.Error("signature should not be empty")
	}

	if !signer.Verify(message, timestamp, sig) {
		t.Error("verification should pass")
	}

	// Different timestamp
	if signer.Verify(message, timestamp+1, sig) {
		t.Error("verification should fail for different timestamp")
	}

	// Different message
	if signer.Verify("Tampered", timestamp, sig) {
		t.Error("verification should fail for different message")
	}
}

func TestTimestampSignerWithHash(t *testing.T) {
	signer := NewTimestampSignerWithHash([]byte("secret-key"), SHA512)

	message := "Hello, World!"
	timestamp := int64(1704067200)

	sig := signer.Sign(message, timestamp)
	if !signer.Verify(message, timestamp, sig) {
		t.Error("verification should pass")
	}
}

func TestAPISigner(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")

	params := map[string]string{
		"user_id": "123",
		"action":  "login",
	}
	timestamp := int64(1704067200)
	nonce := "abc123"

	sig := signer.Sign(params, timestamp, nonce)
	if sig == "" {
		t.Error("signature should not be empty")
	}

	if !signer.Verify(params, timestamp, nonce, sig) {
		t.Error("verification should pass")
	}

	// Different params
	params["user_id"] = "456"
	if signer.Verify(params, timestamp, nonce, sig) {
		t.Error("verification should fail for different params")
	}
}

func TestAPISignerEmptyParams(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")

	params := map[string]string{}
	timestamp := int64(1704067200)
	nonce := "abc123"

	sig := signer.Sign(params, timestamp, nonce)
	if !signer.Verify(params, timestamp, nonce, sig) {
		t.Error("verification should pass for empty params")
	}
}

func TestSortAndJoinParams(t *testing.T) {
	params := map[string]string{
		"c": "3",
		"a": "1",
		"b": "2",
	}

	result := sortAndJoinParams(params)
	expected := "a=1&b=2&c=3"

	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSortAndJoinParamsEmpty(t *testing.T) {
	result := sortAndJoinParams(map[string]string{})
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestFormatInt64(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{123456789, "123456789"},
		{-123456789, "-123456789"},
		{9223372036854775807, "9223372036854775807"},
	}

	for _, tt := range tests {
		result := formatInt64(tt.input)
		if result != tt.expected {
			t.Errorf("formatInt64(%d) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}

func TestDeterministicSignatures(t *testing.T) {
	message := []byte("Hello, World!")
	key := []byte("secret-key")

	sig1 := HMACSHA256(message, key)
	sig2 := HMACSHA256(message, key)

	// HMAC should be deterministic
	if string(sig1) != string(sig2) {
		t.Error("HMAC signatures should be deterministic")
	}
}

func BenchmarkHMACSHA256(b *testing.B) {
	message := []byte("Hello, World! This is a benchmark test message.")
	key := []byte("secret-key-for-benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HMACSHA256(message, key)
	}
}

func BenchmarkHMACSHA512(b *testing.B) {
	message := []byte("Hello, World! This is a benchmark test message.")
	key := []byte("secret-key-for-benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HMACSHA512(message, key)
	}
}

func BenchmarkVerifyHMACSHA256(b *testing.B) {
	message := []byte("Hello, World! This is a benchmark test message.")
	key := []byte("secret-key-for-benchmark")
	sig := HMACSHA256(message, key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyHMACSHA256(message, key, sig)
	}
}
