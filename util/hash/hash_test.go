package hash

import (
	"strings"
	"testing"
)

func TestMD5(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"hello", "5d41402abc4b2a76b9719d911017c592"},
		{"Hello, World!", "65a8e27d8879283831b664bd8b7f0ad4"},
	}

	for _, tt := range tests {
		result := MD5(tt.input)
		if result != tt.expected {
			t.Errorf("MD5(%q) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestMD5Bytes(t *testing.T) {
	data := []byte("hello")
	expected := "5d41402abc4b2a76b9719d911017c592"

	result := MD5Bytes(data)
	if result != expected {
		t.Errorf("MD5Bytes() = %s, expected %s", result, expected)
	}
}

func TestSHA1(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"hello", "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"},
	}

	for _, tt := range tests {
		result := SHA1(tt.input)
		if result != tt.expected {
			t.Errorf("SHA1(%q) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestSHA256(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
	}

	for _, tt := range tests {
		result := SHA256(tt.input)
		if result != tt.expected {
			t.Errorf("SHA256(%q) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestSHA512(t *testing.T) {
	result := SHA512("hello")

	// SHA512 输出128个十六进制字符
	if len(result) != 128 {
		t.Errorf("SHA512 output length = %d, expected 128", len(result))
	}
}

func TestBcryptHash(t *testing.T) {
	password := "mySecretPassword"

	hash, err := BcryptHash(password)
	if err != nil {
		t.Fatalf("BcryptHash failed: %v", err)
	}

	if hash == "" {
		t.Error("BcryptHash returned empty string")
	}

	// Bcrypt hash 应该以 $2a$, $2b$ 或 $2y$ 开头
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
		t.Errorf("invalid bcrypt hash format: %s", hash)
	}
}

func TestBcryptCheck_Success(t *testing.T) {
	password := "mySecretPassword"

	hash, err := BcryptHash(password)
	if err != nil {
		t.Fatalf("BcryptHash failed: %v", err)
	}

	// 正确的密码应该通过验证
	if !BcryptCheck(password, hash) {
		t.Error("BcryptCheck failed for correct password")
	}
}

func TestBcryptCheck_Failure(t *testing.T) {
	password := "mySecretPassword"
	wrongPassword := "wrongPassword"

	hash, err := BcryptHash(password)
	if err != nil {
		t.Fatalf("BcryptHash failed: %v", err)
	}

	// 错误的密码应该验证失败
	if BcryptCheck(wrongPassword, hash) {
		t.Error("BcryptCheck succeeded for wrong password")
	}
}

func TestBcryptHashWithCost(t *testing.T) {
	password := "mySecretPassword"

	tests := []int{4, 6, 8, 10}

	for _, cost := range tests {
		hash, err := BcryptHashWithCost(password, cost)
		if err != nil {
			t.Errorf("BcryptHashWithCost(cost=%d) failed: %v", cost, err)
		}

		if hash == "" {
			t.Errorf("BcryptHashWithCost(cost=%d) returned empty string", cost)
		}

		// 验证密码
		if !BcryptCheck(password, hash) {
			t.Errorf("BcryptCheck failed for cost=%d", cost)
		}
	}
}

func TestBcryptHashWithCost_InvalidCost(t *testing.T) {
	password := "mySecretPassword"

	// Cost 太低
	_, err := BcryptHashWithCost(password, 3)
	if err == nil {
		t.Error("expected error for cost < 4")
	}

	// Cost 太高
	_, err = BcryptHashWithCost(password, 32)
	if err == nil {
		t.Error("expected error for cost > 31")
	}
}

func TestMustBcryptHash(t *testing.T) {
	password := "mySecretPassword"

	hash := MustBcryptHash(password)

	if hash == "" {
		t.Error("MustBcryptHash returned empty string")
	}

	// 验证密码
	if !BcryptCheck(password, hash) {
		t.Error("BcryptCheck failed for MustBcryptHash result")
	}
}

func TestBcryptHash_SameSalts(t *testing.T) {
	password := "mySecretPassword"

	hash1, _ := BcryptHash(password)
	hash2, _ := BcryptHash(password)

	// 同样的密码每次生成的 hash 应该不同（因为 salt 不同）
	if hash1 == hash2 {
		t.Error("BcryptHash generated same hash for same password (salt should be random)")
	}

	// 但两个 hash 都应该能验证同一个密码
	if !BcryptCheck(password, hash1) || !BcryptCheck(password, hash2) {
		t.Error("BcryptCheck failed for same password with different hashes")
	}
}

func TestMD5_EmptyString(t *testing.T) {
	result := MD5("")
	if result == "" {
		t.Error("MD5 should not return empty string for empty input")
	}
}

func TestSHA256_ChineseCharacters(t *testing.T) {
	result := SHA256("你好世界")
	if result == "" {
		t.Error("SHA256 should handle Chinese characters")
	}
	// 验证输出长度
	if len(result) != 64 {
		t.Errorf("SHA256 output length = %d, expected 64", len(result))
	}
}

// Benchmark 测试
func BenchmarkMD5(b *testing.B) {
	data := "hello world"
	for i := 0; i < b.N; i++ {
		MD5(data)
	}
}

func BenchmarkSHA256(b *testing.B) {
	data := "hello world"
	for i := 0; i < b.N; i++ {
		SHA256(data)
	}
}

func BenchmarkBcryptHash(b *testing.B) {
	password := "mySecretPassword"
	for i := 0; i < b.N; i++ {
		BcryptHash(password)
	}
}

func BenchmarkBcryptCheck(b *testing.B) {
	password := "mySecretPassword"
	hash, _ := BcryptHash(password)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BcryptCheck(password, hash)
	}
}

// Additional edge case tests

func TestSHA1Bytes(t *testing.T) {
	data := []byte("hello")
	expected := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"

	result := SHA1Bytes(data)
	if result != expected {
		t.Errorf("SHA1Bytes() = %s, expected %s", result, expected)
	}

	// Test empty bytes
	emptyResult := SHA1Bytes([]byte{})
	if emptyResult == "" {
		t.Error("SHA1Bytes should handle empty bytes")
	}
}

func TestSHA256Bytes(t *testing.T) {
	data := []byte("hello")
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	result := SHA256Bytes(data)
	if result != expected {
		t.Errorf("SHA256Bytes() = %s, expected %s", result, expected)
	}

	// Test empty bytes
	emptyResult := SHA256Bytes([]byte{})
	if emptyResult == "" {
		t.Error("SHA256Bytes should handle empty bytes")
	}
}

func TestSHA512Bytes(t *testing.T) {
	data := []byte("hello")
	result := SHA512Bytes(data)

	// SHA512 输出128个十六进制字符
	if len(result) != 128 {
		t.Errorf("SHA512Bytes output length = %d, expected 128", len(result))
	}

	// Test empty bytes
	emptyResult := SHA512Bytes([]byte{})
	if len(emptyResult) != 128 {
		t.Error("SHA512Bytes should handle empty bytes")
	}
}

func TestMD5_LargeInput(t *testing.T) {
	// 测试大数据输入
	largeData := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 1000)
	result := MD5(largeData)

	if result == "" {
		t.Error("MD5 should handle large input")
	}
	if len(result) != 32 {
		t.Errorf("MD5 output length = %d, expected 32", len(result))
	}
}

func TestSHA256_SpecialCharacters(t *testing.T) {
	tests := []string{
		"!@#$%^&*()",
		"\n\t\r",
		"😀😃😄",
		"你好世界",
		"Привет мир",
		"مرحبا بالعالم",
	}

	for _, input := range tests {
		result := SHA256(input)
		if result == "" {
			t.Errorf("SHA256 should handle special characters: %q", input)
		}
		if len(result) != 64 {
			t.Errorf("SHA256(%q) output length = %d, expected 64", input, len(result))
		}
	}
}

func TestBcryptHashWithCost_BoundaryCosts(t *testing.T) {
	password := "mySecretPassword"

	// Test minimum valid cost
	minHash, err := BcryptHashWithCost(password, 4)
	if err != nil {
		t.Errorf("BcryptHashWithCost(cost=4) should succeed: %v", err)
	}
	if !BcryptCheck(password, minHash) {
		t.Error("BcryptCheck failed for minimum cost")
	}

	// Test maximum valid cost (注意：cost=31 非常慢，使用较低的值测试)
	midHash, err := BcryptHashWithCost(password, 12)
	if err != nil {
		t.Errorf("BcryptHashWithCost(cost=12) should succeed: %v", err)
	}
	if !BcryptCheck(password, midHash) {
		t.Error("BcryptCheck failed for cost=12")
	}
}

func TestBcryptHashWithCost_InvalidCostMessages(t *testing.T) {
	password := "mySecretPassword"

	// Cost 太低 - 验证错误消息
	_, err := BcryptHashWithCost(password, 3)
	if err == nil {
		t.Error("expected error for cost < 4")
	}
	if !strings.Contains(err.Error(), "invalid cost") {
		t.Errorf("error message should contain 'invalid cost', got: %v", err)
	}

	// Cost 太高 - 验证错误消息
	_, err = BcryptHashWithCost(password, 32)
	if err == nil {
		t.Error("expected error for cost > 31")
	}
	if !strings.Contains(err.Error(), "invalid cost") {
		t.Errorf("error message should contain 'invalid cost', got: %v", err)
	}

	// Cost 为 0
	_, err = BcryptHashWithCost(password, 0)
	if err == nil {
		t.Error("expected error for cost = 0")
	}

	// Cost 为负数
	_, err = BcryptHashWithCost(password, -1)
	if err == nil {
		t.Error("expected error for negative cost")
	}
}

func TestBcryptCheck_InvalidHash(t *testing.T) {
	password := "mySecretPassword"

	// 无效的哈希格式
	invalidHashes := []string{
		"",
		"invalid",
		"$2a$10$",
		"not-a-bcrypt-hash",
		"$2a$10$invalidhashformat",
	}

	for _, hash := range invalidHashes {
		if BcryptCheck(password, hash) {
			t.Errorf("BcryptCheck should fail for invalid hash: %q", hash)
		}
	}
}

func TestBcryptCheck_EmptyPassword(t *testing.T) {
	// 测试空密码
	hash, err := BcryptHash("")
	if err != nil {
		t.Fatalf("BcryptHash failed for empty password: %v", err)
	}

	if !BcryptCheck("", hash) {
		t.Error("BcryptCheck should succeed for empty password")
	}

	if BcryptCheck("nonempty", hash) {
		t.Error("BcryptCheck should fail for wrong password")
	}
}

func TestBcryptHash_SpecialCharacters(t *testing.T) {
	passwords := []string{
		"你好世界123Aa",
		"P@ssw0rd!#$%",
		"😀😃😄Aa123",
		"Пароль123",
		"كلمة السر123Aa",
		"\n\t\rAa123456",
	}

	for _, password := range passwords {
		hash, err := BcryptHash(password)
		if err != nil {
			t.Errorf("BcryptHash failed for password with special chars %q: %v", password, err)
			continue
		}

		if !BcryptCheck(password, hash) {
			t.Errorf("BcryptCheck failed for password with special chars: %q", password)
		}

		// 验证不同的密码不会匹配
		if BcryptCheck(password+"x", hash) {
			t.Errorf("BcryptCheck should fail for modified password: %q", password)
		}
	}
}

func TestBcryptHash_LongPassword(t *testing.T) {
	// Bcrypt 对密码长度有限制（72字节）
	// 测试在限制范围内的长密码
	longPassword := strings.Repeat("a", 60) + "Aa1"

	hash, err := BcryptHash(longPassword)
	if err != nil {
		t.Fatalf("BcryptHash failed for long password: %v", err)
	}

	if !BcryptCheck(longPassword, hash) {
		t.Error("BcryptCheck failed for long password")
	}

	// 测试超过限制的密码会返回错误
	tooLongPassword := strings.Repeat("a", 100) + "Aa1"
	_, err = BcryptHash(tooLongPassword)
	if err == nil {
		t.Error("BcryptHash should fail for password exceeding 72 bytes")
	}
}

func TestMustBcryptHash_Success(t *testing.T) {
	// MustBcryptHash 在正常情况下不应该 panic
	password := "validPassword123"

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustBcryptHash should not panic for valid password: %v", r)
		}
	}()

	hash := MustBcryptHash(password)
	if hash == "" {
		t.Error("MustBcryptHash returned empty string")
	}

	if !BcryptCheck(password, hash) {
		t.Error("BcryptCheck failed for MustBcryptHash result")
	}
}

func TestMustBcryptHash_Panic(t *testing.T) {
	// 测试 MustBcryptHash 在错误情况下会 panic
	// 使用超过 72 字节的密码来触发 bcrypt 错误
	tooLongPassword := strings.Repeat("a", 100)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBcryptHash should panic for invalid password")
		}
	}()

	// 这应该会 panic
	MustBcryptHash(tooLongPassword)
}

func TestHashConsistency(t *testing.T) {
	// 测试相同输入产生相同哈希（非 bcrypt）
	input := "test-consistency"

	md5_1 := MD5(input)
	md5_2 := MD5(input)
	if md5_1 != md5_2 {
		t.Error("MD5 should produce consistent results")
	}

	sha1_1 := SHA1(input)
	sha1_2 := SHA1(input)
	if sha1_1 != sha1_2 {
		t.Error("SHA1 should produce consistent results")
	}

	sha256_1 := SHA256(input)
	sha256_2 := SHA256(input)
	if sha256_1 != sha256_2 {
		t.Error("SHA256 should produce consistent results")
	}

	sha512_1 := SHA512(input)
	sha512_2 := SHA512(input)
	if sha512_1 != sha512_2 {
		t.Error("SHA512 should produce consistent results")
	}
}

func TestBytesAndStringConsistency(t *testing.T) {
	// 测试字符串和字节数组版本的一致性
	input := "test-consistency"
	inputBytes := []byte(input)

	if MD5(input) != MD5Bytes(inputBytes) {
		t.Error("MD5 and MD5Bytes should produce same result")
	}

	if SHA1(input) != SHA1Bytes(inputBytes) {
		t.Error("SHA1 and SHA1Bytes should produce same result")
	}

	if SHA256(input) != SHA256Bytes(inputBytes) {
		t.Error("SHA256 and SHA256Bytes should produce same result")
	}

	if SHA512(input) != SHA512Bytes(inputBytes) {
		t.Error("SHA512 and SHA512Bytes should produce same result")
	}
}

// Concurrent testing

func TestBcryptHash_Concurrent(t *testing.T) {
	const goroutines = 10
	const iterations = 5

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			// 确保密码长度不超过 72 字节
			password := strings.Repeat("pw", id+1) + "Aa1"
			for j := 0; j < iterations; j++ {
				hash, err := BcryptHash(password)
				if err != nil {
					t.Errorf("Goroutine %d: BcryptHash failed: %v", id, err)
					done <- false
					return
				}

				if !BcryptCheck(password, hash) {
					t.Errorf("Goroutine %d: BcryptCheck failed", id)
					done <- false
					return
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestMD5_Concurrent(t *testing.T) {
	const goroutines = 20
	const iterations = 100

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			input := strings.Repeat("data", id+1)
			for j := 0; j < iterations; j++ {
				result := MD5(input)
				if result == "" {
					t.Errorf("Goroutine %d: MD5 returned empty string", id)
					done <- false
					return
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestSHA256_Concurrent(t *testing.T) {
	const goroutines = 20
	const iterations = 100

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			input := strings.Repeat("data", id+1)
			for j := 0; j < iterations; j++ {
				result := SHA256(input)
				if len(result) != 64 {
					t.Errorf("Goroutine %d: SHA256 returned invalid length", id)
					done <- false
					return
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
