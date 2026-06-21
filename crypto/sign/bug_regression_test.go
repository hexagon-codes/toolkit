package sign

import "testing"

// Bug2: APISigner.Sign must produce distinct signing strings for distinct
// requests. Without delimiters, sorted-params + timestamp + nonce collide:
//   - {a:"1"} with ts=23  vs  {a:"12"} with ts=3   (param/timestamp boundary)
//   - {a:"1&b=2"}         vs  {a:"1", b:"2"}        (param value vs pair boundary)
func TestBug2_APISigner_NoSigningStringCollision(t *testing.T) {
	s := NewAPISigner("appkey", "secret")

	// Collision class 1: param value bleeds into the timestamp.
	sig1 := s.Sign(map[string]string{"a": "1"}, 23, "n")
	sig2 := s.Sign(map[string]string{"a": "12"}, 3, "n")
	if sig1 == sig2 {
		t.Errorf("distinct requests collided (value/timestamp boundary): %s", sig1)
	}

	// Collision class 2: a param value containing the join chars collides with
	// two separate params.
	sig3 := s.Sign(map[string]string{"a": "1&b=2"}, 100, "n")
	sig4 := s.Sign(map[string]string{"a": "1", "b": "2"}, 100, "n")
	if sig3 == sig4 {
		t.Errorf("distinct requests collided (value vs pair boundary): %s", sig3)
	}

	// Sign/Verify must stay consistent after delimiter changes.
	if !s.Verify(map[string]string{"a": "1"}, 23, "n", sig1) {
		t.Errorf("Verify failed for a freshly produced signature")
	}
	if s.Verify(map[string]string{"a": "12"}, 3, "n", sig1) {
		t.Errorf("Verify accepted a signature for a different request")
	}
}
