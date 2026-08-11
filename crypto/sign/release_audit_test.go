package sign

import "testing"

func TestAPISignerBindsSignatureToAppKey(t *testing.T) {
	first := NewAPISigner("app-a", "shared-secret")
	second := NewAPISigner("app-b", "shared-secret")

	firstSignature := first.Sign(map[string]string{"resource": "1"}, 123, "nonce")
	secondSignature := second.Sign(map[string]string{"resource": "1"}, 123, "nonce")
	if firstSignature == secondSignature {
		t.Fatal("different app keys produced the same API signature")
	}
}

func TestSignerNilReceiversFailClosedWithoutPanicking(t *testing.T) {
	var timestampSigner *TimestampSigner
	if signature := timestampSigner.Sign("message", 1); signature != "" {
		t.Fatalf("nil TimestampSigner.Sign() = %q, want empty", signature)
	}
	if timestampSigner.Verify("message", 1, "signature") {
		t.Fatal("nil TimestampSigner.Verify() accepted a signature")
	}
	if timestampSigner.VerifyWithExpiry("message", 1, "signature", 60) {
		t.Fatal("nil TimestampSigner.VerifyWithExpiry() accepted a signature")
	}

	var apiSigner *APISigner
	if signature := apiSigner.Sign(nil, 1, "nonce"); signature != "" {
		t.Fatalf("nil APISigner.Sign() = %q, want empty", signature)
	}
	if apiSigner.Verify(nil, 1, "nonce", "signature") {
		t.Fatal("nil APISigner.Verify() accepted a signature")
	}
	if apiSigner.VerifyWithExpiry(nil, 1, "nonce", "signature", 60) {
		t.Fatal("nil APISigner.VerifyWithExpiry() accepted a signature")
	}
}

func TestTimestampWindowRejectsFutureTimestamp(t *testing.T) {
	const now int64 = 100
	if timestampWithinAge(now+1, 60, now) {
		t.Fatal("timestampWithinAge() accepted a future timestamp")
	}
}
