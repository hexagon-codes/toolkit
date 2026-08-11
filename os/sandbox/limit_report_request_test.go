package sandbox

import "testing"

func TestRequestedLimitStatusDistinguishesRequestFromBackendCapability(t *testing.T) {
	tests := []struct {
		name       string
		requested  bool
		capability LimitStatus
		want       LimitStatus
	}{
		{name: "not requested on supported backend", capability: LimitStatusEnforced, want: LimitStatusNotRequested},
		{name: "not requested on unsupported backend", capability: LimitStatusUnsupported, want: LimitStatusNotRequested},
		{name: "requested and enforced", requested: true, capability: LimitStatusEnforced, want: LimitStatusEnforced},
		{name: "requested but unsupported", requested: true, capability: LimitStatusUnsupported, want: LimitStatusUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := requestedLimitStatus(test.requested, test.capability); got != test.want {
				t.Fatalf("requestedLimitStatus(%t, %q) = %q, want %q", test.requested, test.capability, got, test.want)
			}
		})
	}
}
