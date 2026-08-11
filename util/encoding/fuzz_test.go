package encoding

import (
	"net/url"
	"testing"
)

func FuzzJoinURLNeverReturnsMalformedURL(f *testing.F) {
	f.Add("https://example.com/api", "v1/users")
	f.Add("/relative", "child")
	f.Fuzz(func(t *testing.T, base, element string) {
		joined, err := JoinURL(base, element)
		if err != nil {
			return
		}
		if _, err := url.Parse(joined); err != nil {
			t.Fatalf("JoinURL() returned malformed URL %q: %v", joined, err)
		}
	})
}

func FuzzBase64RoundTrip(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, input []byte) {
		decoded, err := Base64Decode(Base64Encode(input))
		if err != nil {
			t.Fatalf("Base64Decode() error: %v", err)
		}
		if string(decoded) != string(input) {
			t.Fatal("Base64 round trip changed input")
		}
	})
}
