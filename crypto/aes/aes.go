package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidKeySize 表示 AES 密钥长度不是 16、24 或 32 字节。
	ErrInvalidKeySize = errors.New("aes: invalid key size, must be 16, 24, or 32 bytes")
	// ErrInvalidCiphertext 表示密文长度不足。
	ErrInvalidCiphertext = errors.New("aes: ciphertext too short")
	// ErrAuthenticationFailed 表示密文认证失败。
	ErrAuthenticationFailed = errors.New("aes: authentication failed")
)

// --- GCM 模式（推荐，带认证） ---

// EncryptGCM 使用 AES-GCM 加密（推荐）
// key: 16/24/32 字节对应 AES-128/192/256
// 返回: nonce + ciphertext
func EncryptGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidKeySize, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptGCM 使用 AES-GCM 解密
func DecryptGCM(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidKeySize, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuthenticationFailed, err)
	}

	return plaintext, nil
}

// EncryptGCMString 加密字符串，返回 Base64
func EncryptGCMString(plaintext, key string) (string, error) {
	ciphertext, err := EncryptGCM([]byte(plaintext), []byte(key))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptGCMString 解密 Base64 字符串
func DecryptGCMString(ciphertext, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := DecryptGCM(data, []byte(key))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- 工具函数 ---

// GenerateKey 生成指定长度的随机密钥
// size: 16, 24, 或 32
func GenerateKey(size int) ([]byte, error) {
	if size != 16 && size != 24 && size != 32 {
		return nil, ErrInvalidKeySize
	}
	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateKeyHex 生成密钥并返回 Hex 编码
func GenerateKeyHex(size int) (string, error) {
	key, err := GenerateKey(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// GenerateKeyBase64 生成密钥并返回 Base64 编码
func GenerateKeyBase64(size int) (string, error) {
	key, err := GenerateKey(size)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// --- 安全工具函数 ---

// ClearBytes 安全清除字节切片内容
// 用于清除密钥等敏感数据，防止内存残留
//
// 注意事项：
//   - Go 的 GC 可能已经复制了数据到其他位置，此函数只能尽力而为
//   - 建议在使用完密钥后立即调用此函数
//   - 对于极高安全要求的场景，考虑使用专门的安全内存库
//
//go:noinline
func ClearBytes(b []byte) {
	// 使用显式循环而非 copy/memset 确保每个字节都被清零
	// go:noinline 指令防止编译器内联此函数，从而保留清零操作
	for i := range b {
		b[i] = 0
	}
}
