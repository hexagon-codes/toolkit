package hash

import (
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestSHA256(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "", expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{input: "hello", expected: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{input: "你好世界", expected: "beca6335b20ff57ccc47403ef4d9e0b8fccb4442b3151c2e7d50050673d43172"},
	}
	for _, test := range tests {
		if actual := SHA256(test.input); actual != test.expected {
			t.Errorf("SHA256(%q) = %s, want %s", test.input, actual, test.expected)
		}
		if actual := SHA256Bytes([]byte(test.input)); actual != test.expected {
			t.Errorf("SHA256Bytes(%q) = %s, want %s", test.input, actual, test.expected)
		}
	}
}

func TestSHA512(t *testing.T) {
	for _, input := range []string{"", "hello", "你好世界"} {
		stringHash := SHA512(input)
		bytesHash := SHA512Bytes([]byte(input))
		if len(stringHash) != 128 || stringHash != bytesHash {
			t.Errorf("SHA512(%q) length = %d, bytes equal = %v", input, len(stringHash), stringHash == bytesHash)
		}
	}
}

func TestBcryptHashAndCheck(t *testing.T) {
	password := "mySecretPassword"
	first, err := BcryptHash(password)
	if err != nil {
		t.Fatalf("BcryptHash() error = %v", err)
	}
	second, err := BcryptHash(password)
	if err != nil {
		t.Fatalf("second BcryptHash() error = %v", err)
	}
	if first == second {
		t.Fatal("BcryptHash() reused a salt")
	}
	if !BcryptCheck(password, first) || BcryptCheck("wrong", first) {
		t.Fatal("BcryptCheck() returned an invalid result")
	}
	if BcryptCheck(password, "not-a-hash") {
		t.Fatal("BcryptCheck() accepted an invalid hash")
	}
}

func TestBcryptHashWithCost(t *testing.T) {
	for _, cost := range []int{bcrypt.MinCost, bcrypt.DefaultCost} {
		hash, err := BcryptHashWithCost("password", cost)
		if err != nil || !BcryptCheck("password", hash) {
			t.Fatalf("BcryptHashWithCost(%d) = (%q, %v)", cost, hash, err)
		}
	}
	for _, cost := range []int{bcrypt.MinCost - 1, bcrypt.MaxCost + 1} {
		if _, err := BcryptHashWithCost("password", cost); err == nil {
			t.Fatalf("BcryptHashWithCost(%d) error = nil", cost)
		}
	}
	if _, err := BcryptHash(strings.Repeat("a", 73)); err == nil {
		t.Fatal("BcryptHash() accepted a password longer than 72 bytes")
	}
}

func TestMustBcryptHash(t *testing.T) {
	hash := MustBcryptHash("password")
	if !BcryptCheck("password", hash) {
		t.Fatal("MustBcryptHash() result did not verify")
	}
	deferredPanic := func() (panicked bool) {
		defer func() { panicked = recover() != nil }()
		_ = MustBcryptHash(strings.Repeat("a", 73))
		return false
	}()
	if !deferredPanic {
		t.Fatal("MustBcryptHash() did not panic on invalid input")
	}
}

func TestHashFunctionsAreConcurrentSafe(t *testing.T) {
	const workers = 32
	var waitGroup sync.WaitGroup
	for worker := range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			input := strings.Repeat("data", worker+1)
			if SHA256(input) != SHA256Bytes([]byte(input)) {
				t.Errorf("SHA256 variants differ for worker %d", worker)
			}
			if SHA512(input) != SHA512Bytes([]byte(input)) {
				t.Errorf("SHA512 variants differ for worker %d", worker)
			}
		}()
	}
	waitGroup.Wait()
}

func BenchmarkSHA256(b *testing.B) {
	for b.Loop() {
		_ = SHA256("hello world")
	}
}

func BenchmarkBcryptHash(b *testing.B) {
	for b.Loop() {
		_, _ = BcryptHash("mySecretPassword")
	}
}
