package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

var (
	ErrInvalidKeySize    = errors.New("aes: invalid key size, must be 16, 24, or 32 bytes")
	ErrInvalidBlockSize  = errors.New("aes: invalid block size")
	ErrInvalidCiphertext = errors.New("aes: ciphertext too short")
	ErrInvalidPadding    = errors.New("aes: invalid padding")
)

// --- GCM 模式（推荐，带认证） ---

// EncryptGCM 使用 AES-GCM 加密（推荐）
// key: 16/24/32 字节对应 AES-128/192/256
// 返回: nonce + ciphertext
func EncryptGCM(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeySize
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
		return nil, ErrInvalidKeySize
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
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

// --- CBC 模式 ---

// EncryptCBC 使用 AES-CBC 加密
// key: 16/24/32 字节
// 返回: iv + ciphertext
//
// 警告: CBC 模式不提供消息认证，易受填充预言攻击
// 推荐使用 EncryptGCM 替代，除非有特殊的兼容性需求
func EncryptCBC(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeySize
	}

	// PKCS7 填充
	plaintext = pkcs7Pad(plaintext, block.BlockSize())

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

// DecryptCBC 使用 AES-CBC 解密
func DecryptCBC(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeySize
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, ErrInvalidCiphertext
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, ErrInvalidBlockSize
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// PKCS7 去填充
	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptCBCString 加密字符串，返回 Base64
func EncryptCBCString(plaintext, key string) (string, error) {
	ciphertext, err := EncryptCBC([]byte(plaintext), []byte(key))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCBCString 解密 Base64 字符串
func DecryptCBCString(ciphertext, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := DecryptCBC(data, []byte(key))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- CTR 模式（流加密） ---

// EncryptCTR 使用 AES-CTR 加密
//
// 警告: CTR 模式不提供消息认证，可能遭受位翻转攻击
// 推荐使用 EncryptGCM 替代，除非有特殊的兼容性需求
func EncryptCTR(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeySize
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

// DecryptCTR 使用 AES-CTR 解密
func DecryptCTR(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeySize
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, ErrInvalidCiphertext
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	plaintext := make([]byte, len(ciphertext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
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
// 注意：Go 的 GC 可能已经复制了数据，此函数只能尽力而为
func ClearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// --- PKCS7 填充 ---

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidPadding
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, ErrInvalidPadding
	}

	// 使用恒定时间比较防止时序攻击
	// 始终检查所有填充字节，不提前返回
	valid := 1
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			valid = 0
		}
	}

	if valid == 0 {
		return nil, ErrInvalidPadding
	}
	return data[:len(data)-padding], nil
}
