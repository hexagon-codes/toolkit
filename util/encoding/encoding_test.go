package encoding

import (
	"testing"
)

// Base64 Tests

func TestBase64Encode(t *testing.T) {
	data := []byte("Hello, World!")
	encoded := Base64Encode(data)
	expected := "SGVsbG8sIFdvcmxkIQ=="

	if encoded != expected {
		t.Errorf("expected %s, got %s", expected, encoded)
	}
}

func TestBase64EncodeString(t *testing.T) {
	encoded := Base64EncodeString("Hello, World!")
	expected := "SGVsbG8sIFdvcmxkIQ=="

	if encoded != expected {
		t.Errorf("expected %s, got %s", expected, encoded)
	}
}

func TestBase64Decode(t *testing.T) {
	decoded, err := Base64Decode("SGVsbG8sIFdvcmxkIQ==")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if string(decoded) != "Hello, World!" {
		t.Errorf("expected Hello, World!, got %s", string(decoded))
	}

	// Invalid input
	_, err = Base64Decode("invalid!!!")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestBase64DecodeString(t *testing.T) {
	decoded, err := Base64DecodeString("SGVsbG8sIFdvcmxkIQ==")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if decoded != "Hello, World!" {
		t.Errorf("expected Hello, World!, got %s", decoded)
	}
}

func TestBase64URLEncode(t *testing.T) {
	// URL encoding uses - and _ instead of + and /
	data := []byte{0xfb, 0xff} // Would be +/ in standard base64
	encoded := Base64URLEncode(data)
	standard := Base64Encode(data)

	if encoded == standard {
		t.Error("URL encoding should differ from standard for this input")
	}
}

func TestBase64URLDecode(t *testing.T) {
	encoded := Base64URLEncodeString("Hello")
	decoded, err := Base64URLDecodeString(encoded)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if decoded != "Hello" {
		t.Errorf("expected Hello, got %s", decoded)
	}
}

func TestBase64RawEncode(t *testing.T) {
	// Raw encoding has no padding
	encoded := Base64RawEncode([]byte("a"))
	if encoded == Base64Encode([]byte("a")) {
		t.Error("Raw encoding should have no padding")
	}

	// Decode should work
	decoded, err := Base64RawDecode(encoded)
	if err != nil || string(decoded) != "a" {
		t.Error("Raw decode failed")
	}
}

func TestBase64RawURLEncode(t *testing.T) {
	encoded := Base64RawURLEncode([]byte("Hello"))
	decoded, err := Base64RawURLDecode(encoded)
	if err != nil || string(decoded) != "Hello" {
		t.Error("Raw URL encode/decode failed")
	}
}

// Hex Tests

func TestHexEncode(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	encoded := HexEncode(data)

	if encoded != "deadbeef" {
		t.Errorf("expected deadbeef, got %s", encoded)
	}
}

func TestHexEncodeString(t *testing.T) {
	encoded := HexEncodeString("Hi")
	if encoded != "4869" {
		t.Errorf("expected 4869, got %s", encoded)
	}
}

func TestHexDecode(t *testing.T) {
	decoded, err := HexDecode("deadbeef")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expected := []byte{0xde, 0xad, 0xbe, 0xef}
	for i, b := range decoded {
		if b != expected[i] {
			t.Errorf("mismatch at %d: expected %x, got %x", i, expected[i], b)
		}
	}

	// Invalid input
	_, err = HexDecode("xyz")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestHexDecodeString(t *testing.T) {
	decoded, err := HexDecodeString("4869")
	if err != nil || decoded != "Hi" {
		t.Error("HexDecodeString failed")
	}

	// Invalid input
	_, err = HexDecodeString("xyz")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestHexEncodeUpper(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	encoded := HexEncodeUpper(data)

	if encoded != "DEADBEEF" {
		t.Errorf("expected DEADBEEF, got %s", encoded)
	}
}

// URL Tests

func TestURLEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello+world"},
		{"foo=bar", "foo%3Dbar"},
		{"a/b", "a%2Fb"},
	}

	for _, tt := range tests {
		encoded := URLEncode(tt.input)
		if encoded != tt.expected {
			t.Errorf("URLEncode(%s) = %s, want %s", tt.input, encoded, tt.expected)
		}
	}
}

func TestURLDecode(t *testing.T) {
	decoded, err := URLDecode("hello+world")
	if err != nil || decoded != "hello world" {
		t.Error("URLDecode failed")
	}

	decoded, err = URLDecode("foo%3Dbar")
	if err != nil || decoded != "foo=bar" {
		t.Error("URLDecode failed for percent-encoded")
	}

	// Invalid
	_, err = URLDecode("%")
	if err == nil {
		t.Error("expected error for invalid encoding")
	}
}

func TestURLPathEncode(t *testing.T) {
	encoded := URLPathEncode("hello world")
	if encoded != "hello%20world" {
		t.Errorf("expected hello%%20world, got %s", encoded)
	}
}

func TestURLPathDecode(t *testing.T) {
	decoded, err := URLPathDecode("hello%20world")
	if err != nil || decoded != "hello world" {
		t.Error("URLPathDecode failed")
	}
}

func TestBuildQuery(t *testing.T) {
	params := map[string]string{
		"name": "John Doe",
		"age":  "30",
	}

	query := BuildQuery(params)

	// Order is not guaranteed
	if query != "name=John+Doe&age=30" && query != "age=30&name=John+Doe" {
		t.Errorf("unexpected query string: %s", query)
	}

	// Empty map
	if BuildQuery(map[string]string{}) != "" {
		t.Error("empty map should return empty string")
	}
}

func TestParseQuery(t *testing.T) {
	params, err := ParseQuery("name=John&age=30")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if params["name"] != "John" || params["age"] != "30" {
		t.Error("ParseQuery failed")
	}

	// Invalid query
	_, err = ParseQuery("%")
	if err == nil {
		t.Error("expected error for invalid query")
	}
}

func TestParseQueryValues(t *testing.T) {
	// Multiple values for same key
	params, err := ParseQueryValues("key=a&key=b&key=c")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(params["key"]) != 3 {
		t.Error("ParseQueryValues should handle multiple values")
	}
}

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base     string
		paths    []string
		expected string
	}{
		{"https://example.com", []string{"api", "v1"}, "https://example.com/api/v1"},
		{"https://example.com/", []string{"/api/", "/v1/"}, "https://example.com/api/v1"},
		{"https://example.com", []string{}, "https://example.com"},
		{"https://example.com", []string{""}, "https://example.com"},
		{"https://example.com/api", []string{"users"}, "https://example.com/api/users"},
	}

	for _, tt := range tests {
		result := JoinURL(tt.base, tt.paths...)
		if result != tt.expected {
			t.Errorf("JoinURL(%s, %v) = %s, want %s", tt.base, tt.paths, result, tt.expected)
		}
	}
}
