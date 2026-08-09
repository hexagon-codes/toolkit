package sign

import (
	"math"
	"testing"
	"time"
)

type testNonceChecker struct {
	called   bool
	expireAt int64
}

func (c *testNonceChecker) Check(_ string, expireAt int64) bool {
	c.called = true
	c.expireAt = expireAt
	return true
}

func TestTimestampVerificationRejectsOverflowingAncientTimestamp(t *testing.T) {
	timestampSigner := NewTimestampSigner([]byte("timestamp-secret"))
	timestamp := int64(math.MinInt64)
	timestampSignature := timestampSigner.Sign("message", timestamp)
	if timestampSigner.VerifyWithExpiry("message", timestamp, timestampSignature, 300) {
		t.Fatal("TimestampSigner accepted an ancient timestamp after integer overflow")
	}

	apiSigner := NewAPISigner("app-key", "app-secret")
	apiSignature := apiSigner.Sign(nil, timestamp, "nonce")
	if apiSigner.VerifyWithExpiry(nil, timestamp, "nonce", apiSignature, 300) {
		t.Fatal("APISigner accepted an ancient timestamp after integer overflow")
	}
}

func TestVerifyWithNonceCheckRejectsNilCheckerWithoutPanicking(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")
	timestamp := time.Now().Unix()
	signature := signer.Sign(nil, timestamp, "nonce")
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("VerifyWithNonceCheck panicked: %v", recovered)
		}
	}()

	if signer.VerifyWithNonceCheck(nil, timestamp, "nonce", signature, 300, nil) {
		t.Fatal("VerifyWithNonceCheck accepted a nil nonce checker")
	}
}

func TestVerifyWithNonceCheckRejectsTypedNilCheckerWithoutPanicking(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")
	timestamp := time.Now().Unix()
	signature := signer.Sign(nil, timestamp, "nonce")
	var checker *testNonceChecker
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("VerifyWithNonceCheck panicked: %v", recovered)
		}
	}()

	if signer.VerifyWithNonceCheck(nil, timestamp, "nonce", signature, 300, checker) {
		t.Fatal("VerifyWithNonceCheck accepted a typed nil nonce checker")
	}
}

func TestVerifyWithNonceCheckRejectsEmptyNonce(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")
	timestamp := time.Now().Unix()
	signature := signer.Sign(nil, timestamp, "")
	checker := &testNonceChecker{}

	if signer.VerifyWithNonceCheck(nil, timestamp, "", signature, 300, checker) {
		t.Fatal("VerifyWithNonceCheck accepted an empty nonce")
	}
	if checker.called {
		t.Fatal("VerifyWithNonceCheck called the nonce store for an empty nonce")
	}
}

func TestVerifyWithNonceCheckRejectsExpiryOverflow(t *testing.T) {
	signer := NewAPISigner("app-key", "app-secret")
	timestamp := time.Now().Unix()
	signature := signer.Sign(nil, timestamp, "nonce")
	checker := &testNonceChecker{}

	if signer.VerifyWithNonceCheck(nil, timestamp, "nonce", signature, math.MaxInt64, checker) {
		t.Fatal("VerifyWithNonceCheck accepted an overflowing nonce expiry")
	}
	if checker.called {
		t.Fatalf("nonce checker received overflowing expiry %d", checker.expireAt)
	}
}

func TestHMACRejectsUnknownHashInsteadOfDowngrading(t *testing.T) {
	message := []byte("message")
	key := []byte("secret")
	unknown := HMACHash(99)

	if signature := HMAC(message, key, unknown); signature != nil {
		t.Fatalf("HMAC returned %x for an unknown hash", signature)
	}
	sha256Signature := HMAC(message, key, SHA256)
	if VerifyHMAC(message, key, sha256Signature, unknown) {
		t.Fatal("VerifyHMAC downgraded an unknown hash to SHA-256")
	}
}

func TestTimestampSignerOwnsKeyMaterial(t *testing.T) {
	key := []byte("secret-key")
	signer := NewTimestampSigner(key)
	before := signer.Sign("message", 1)
	for index := range key {
		key[index] = 0
	}
	after := signer.Sign("message", 1)

	if before != after {
		t.Fatal("TimestampSigner signature changed after caller mutated the key")
	}
}
