package rsa

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

var (
	// ErrInvalidKeySize 表示 RSA 密钥位数低于安全下限。
	ErrInvalidKeySize = errors.New("rsa: invalid key size, minimum 2048 bits")
	// ErrInvalidPublicKey 表示公钥无效。
	ErrInvalidPublicKey = errors.New("rsa: invalid public key")
	// ErrInvalidPrivateKey 表示私钥无效。
	ErrInvalidPrivateKey = errors.New("rsa: invalid private key")
	// ErrInvalidPEMBlock 表示 PEM 数据无效。
	ErrInvalidPEMBlock = errors.New("rsa: invalid PEM block")
	// ErrDecryptionFailed 表示解密失败。
	ErrDecryptionFailed = errors.New("rsa: decryption failed")
	// ErrMessageTooLong 表示消息超过密钥可处理长度。
	ErrMessageTooLong = errors.New("rsa: message too long for key size")
	// ErrInvalidSignature 表示签名校验失败。
	ErrInvalidSignature = errors.New("rsa: invalid signature")
)

// KeyPair RSA 密钥对
type KeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

// GenerateKeyPair 生成 RSA 密钥对
// bits: 建议 2048 或 4096（最小 2048 位，1024 位已不安全）
func GenerateKeyPair(bits int) (*KeyPair, error) {
	if bits < 2048 {
		return nil, ErrInvalidKeySize
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}, nil
}

// PrivateKeyToPEM 私钥转 PEM 格式。
func (kp *KeyPair) PrivateKeyToPEM() (string, error) {
	if kp == nil {
		return "", ErrInvalidPrivateKey
	}
	if err := validatePrivateKey(kp.PrivateKey); err != nil {
		return "", err
	}
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(kp.PrivateKey)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

// PublicKeyToPEM 公钥转 PEM 格式。
func (kp *KeyPair) PublicKeyToPEM() (string, error) {
	if kp == nil {
		return "", ErrInvalidPublicKey
	}
	if err := validatePublicKey(kp.PublicKey); err != nil {
		return "", err
	}
	publicKeyBytes := x509.MarshalPKCS1PublicKey(kp.PublicKey)
	block := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

// PrivateKeyToPKCS8PEM 私钥转 PKCS8 PEM 格式
func (kp *KeyPair) PrivateKeyToPKCS8PEM() (string, error) {
	if kp == nil {
		return "", ErrInvalidPrivateKey
	}
	if err := validatePrivateKey(kp.PrivateKey); err != nil {
		return "", err
	}
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(kp.PrivateKey)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

// PublicKeyToPKIXPEM 公钥转 PKIX PEM 格式
func (kp *KeyPair) PublicKeyToPKIXPEM() (string, error) {
	if kp == nil {
		return "", ErrInvalidPublicKey
	}
	if err := validatePublicKey(kp.PublicKey); err != nil {
		return "", err
	}
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(kp.PublicKey)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

// --- 解析 PEM ---

// ParsePrivateKey 从 PEM 解析私钥
func ParsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, err := decodePEMBlock(pemData)
	if err != nil {
		return nil, err
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		privateKey, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidPrivateKey, parseErr)
		}
		if validationErr := validatePrivateKey(privateKey); validationErr != nil {
			return nil, validationErr
		}
		return privateKey, nil
	case "PRIVATE KEY":
		// 继续解析 PKCS8。
	default:
		return nil, fmt.Errorf("%w: unexpected type %q", ErrInvalidPEMBlock, block.Type)
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPrivateKey, err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidPrivateKey
	}
	if err := validatePrivateKey(rsaKey); err != nil {
		return nil, err
	}

	return rsaKey, nil
}

// ParsePublicKey 从 PEM 解析公钥
func ParsePublicKey(pemData string) (*rsa.PublicKey, error) {
	block, err := decodePEMBlock(pemData)
	if err != nil {
		return nil, err
	}

	switch block.Type {
	case "RSA PUBLIC KEY":
		publicKey, parseErr := x509.ParsePKCS1PublicKey(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidPublicKey, parseErr)
		}
		if validationErr := validatePublicKey(publicKey); validationErr != nil {
			return nil, validationErr
		}
		return publicKey, nil
	case "PUBLIC KEY":
		// 继续解析 PKIX。
	default:
		return nil, fmt.Errorf("%w: unexpected type %q", ErrInvalidPEMBlock, block.Type)
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPublicKey, err)
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPublicKey
	}
	if err := validatePublicKey(rsaKey); err != nil {
		return nil, err
	}

	return rsaKey, nil
}

func decodePEMBlock(pemData string) (*pem.Block, error) {
	block, rest := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ErrInvalidPEMBlock
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("%w: trailing data", ErrInvalidPEMBlock)
	}
	return block, nil
}

func validatePublicKey(publicKey *rsa.PublicKey) error {
	if publicKey == nil || publicKey.N == nil || publicKey.N.Sign() <= 0 {
		return ErrInvalidPublicKey
	}
	if publicKey.N.Bit(0) == 0 {
		return fmt.Errorf("%w: modulus must be odd", ErrInvalidPublicKey)
	}
	if publicKey.E < 3 || publicKey.E%2 == 0 || publicKey.E > 1<<31-1 {
		return fmt.Errorf("%w: exponent is outside the valid range", ErrInvalidPublicKey)
	}
	if publicKey.N.BitLen() < 2048 {
		return fmt.Errorf("%w: %w: modulus has %d bits", ErrInvalidPublicKey, ErrInvalidKeySize, publicKey.N.BitLen())
	}
	return nil
}

func validatePrivateKey(privateKey *rsa.PrivateKey) error {
	if privateKey == nil {
		return ErrInvalidPrivateKey
	}
	if err := validatePublicKey(&privateKey.PublicKey); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPrivateKey, err)
	}
	if err := privateKey.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPrivateKey, err)
	}
	return nil
}

// --- OAEP 加解密（推荐） ---

// EncryptOAEP 使用 OAEP 填充加密（推荐）
func EncryptOAEP(plaintext []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	if err := validatePublicKey(publicKey); err != nil {
		return nil, err
	}
	maximum := publicKey.Size() - 2*sha256.Size - 2
	if len(plaintext) > maximum {
		return nil, fmt.Errorf("%w: maximum %d bytes", ErrMessageTooLong, maximum)
	}
	hash := sha256.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, plaintext, nil)
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

// DecryptOAEP 使用 OAEP 填充解密
func DecryptOAEP(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	if err := validatePrivateKey(privateKey); err != nil {
		return nil, err
	}
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		// 对所有密文格式、长度与填充失败仅暴露一个公共错误，避免形成解密 oracle。
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// EncryptOAEPString 加密字符串，返回 Base64
func EncryptOAEPString(plaintext, publicKeyPEM string) (string, error) {
	publicKey, err := ParsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}
	ciphertext, err := EncryptOAEP([]byte(plaintext), publicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptOAEPString 解密 Base64 字符串
func DecryptOAEPString(ciphertext, privateKeyPEM string) (string, error) {
	privateKey, err := ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := DecryptOAEP(data, privateKey)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- 签名与验证 ---

// SignPSS 使用 PSS 签名（推荐）
func SignPSS(message []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	if err := validatePrivateKey(privateKey); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(message)
	options := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	return rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], options)
}

// VerifyPSS 验证 PSS 签名
func VerifyPSS(message, signature []byte, publicKey *rsa.PublicKey) error {
	if err := validatePublicKey(publicKey); err != nil {
		return err
	}
	hash := sha256.Sum256(message)
	options := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	if err := rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signature, options); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}
	return nil
}

// SignString 签名字符串，返回 Base64
func SignString(message, privateKeyPEM string) (string, error) {
	privateKey, err := ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	signature, err := SignPSS([]byte(message), privateKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyString 验证签名
func VerifyString(message, signature, publicKeyPEM string) error {
	publicKey, err := ParsePublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	return VerifyPSS([]byte(message), sig, publicKey)
}

// --- 便捷方法 ---

// Encrypt 使用 KeyPair 加密
func (kp *KeyPair) Encrypt(plaintext []byte) ([]byte, error) {
	if kp == nil {
		return nil, ErrInvalidPublicKey
	}
	return EncryptOAEP(plaintext, kp.PublicKey)
}

// Decrypt 使用 KeyPair 解密
func (kp *KeyPair) Decrypt(ciphertext []byte) ([]byte, error) {
	if kp == nil {
		return nil, ErrInvalidPrivateKey
	}
	return DecryptOAEP(ciphertext, kp.PrivateKey)
}

// Sign 使用 KeyPair 签名
func (kp *KeyPair) Sign(message []byte) ([]byte, error) {
	if kp == nil {
		return nil, ErrInvalidPrivateKey
	}
	return SignPSS(message, kp.PrivateKey)
}

// Verify 使用 KeyPair 验签
func (kp *KeyPair) Verify(message, signature []byte) error {
	if kp == nil {
		return ErrInvalidPublicKey
	}
	return VerifyPSS(message, signature, kp.PublicKey)
}
