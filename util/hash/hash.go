package hash

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// SHA256 计算 SHA256 哈希
func SHA256(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// SHA256Bytes 计算字节数组的 SHA256 哈希
func SHA256Bytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// SHA512 计算 SHA512 哈希
func SHA512(data string) string {
	hash := sha512.Sum512([]byte(data))
	return hex.EncodeToString(hash[:])
}

// SHA512Bytes 计算字节数组的 SHA512 哈希
func SHA512Bytes(data []byte) string {
	hash := sha512.Sum512(data)
	return hex.EncodeToString(hash[:])
}

// BcryptHash 使用 bcrypt 加密密码
// bcrypt 拒绝超过 72 字节的密码，调用方应限制输入长度。
func BcryptHash(password string) (string, error) {
	return BcryptHashWithCost(password, bcrypt.DefaultCost)
}

// BcryptHashWithCost 使用指定 cost 加密密码
func BcryptHashWithCost(password string, cost int) (string, error) {
	// 验证 cost 范围 (bcrypt.MinCost=4, bcrypt.MaxCost=31)
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return "", fmt.Errorf("invalid cost: must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// BcryptCheck 验证密码
func BcryptCheck(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// MustBcryptHash bcrypt 加密，失败时 panic
func MustBcryptHash(password string) string {
	hash, err := BcryptHash(password)
	if err != nil {
		panic(err)
	}
	return hash
}
