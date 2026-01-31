package rsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
)

var (
	ErrInvalidKeySize    = errors.New("rsa: invalid key size, minimum 2048 bits")
	ErrInvalidPublicKey  = errors.New("rsa: invalid public key")
	ErrInvalidPrivateKey = errors.New("rsa: invalid private key")
	ErrInvalidPEMBlock   = errors.New("rsa: invalid PEM block")
	ErrDecryptionFailed  = errors.New("rsa: decryption failed")
	ErrMessageTooLong    = errors.New("rsa: message too long for key size")
	ErrInvalidSignature  = errors.New("rsa: invalid signature")
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

// PrivateKeyToPEM 私钥转 PEM 格式
func (kp *KeyPair) PrivateKeyToPEM() string {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(kp.PrivateKey)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	return string(pem.EncodeToMemory(block))
}

// PublicKeyToPEM 公钥转 PEM 格式
func (kp *KeyPair) PublicKeyToPEM() string {
	publicKeyBytes := x509.MarshalPKCS1PublicKey(kp.PublicKey)
	block := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	return string(pem.EncodeToMemory(block))
}

// PrivateKeyToPKCS8PEM 私钥转 PKCS8 PEM 格式
func (kp *KeyPair) PrivateKeyToPKCS8PEM() (string, error) {
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
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ErrInvalidPEMBlock
	}

	// 尝试 PKCS1 格式
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	// 尝试 PKCS8 格式
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidPrivateKey
	}

	return rsaKey, nil
}

// ParsePublicKey 从 PEM 解析公钥
func ParsePublicKey(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ErrInvalidPEMBlock
	}

	// 尝试 PKCS1 格式
	if block.Type == "RSA PUBLIC KEY" {
		return x509.ParsePKCS1PublicKey(block.Bytes)
	}

	// 尝试 PKIX 格式
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidPublicKey
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPublicKey
	}

	return rsaKey, nil
}

// --- OAEP 加解密（推荐） ---

// EncryptOAEP 使用 OAEP 填充加密（推荐）
func EncryptOAEP(plaintext []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	hash := sha256.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, plaintext, nil)
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

// DecryptOAEP 使用 OAEP 填充解密
func DecryptOAEP(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// EncryptOAEPString 加密字符串，返回 Base64
func EncryptOAEPString(plaintext string, publicKeyPEM string) (string, error) {
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
func DecryptOAEPString(ciphertext string, privateKeyPEM string) (string, error) {
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

// --- PKCS1v15 加解密 ---

// EncryptPKCS1v15 使用 PKCS1v15 填充加密
//
// 警告: PKCS1v15 易受填充预言攻击，推荐使用 EncryptOAEP 替代
// 仅在需要兼容旧系统时使用此函数
func EncryptPKCS1v15(plaintext []byte, publicKey *rsa.PublicKey) ([]byte, error) {
	return rsa.EncryptPKCS1v15(rand.Reader, publicKey, plaintext)
}

// DecryptPKCS1v15 使用 PKCS1v15 填充解密
func DecryptPKCS1v15(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	return rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
}

// EncryptPKCS1v15String 加密字符串，返回 Base64
func EncryptPKCS1v15String(plaintext string, publicKeyPEM string) (string, error) {
	publicKey, err := ParsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}
	ciphertext, err := EncryptPKCS1v15([]byte(plaintext), publicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPKCS1v15String 解密 Base64 字符串
func DecryptPKCS1v15String(ciphertext string, privateKeyPEM string) (string, error) {
	privateKey, err := ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := DecryptPKCS1v15(data, privateKey)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// --- 签名与验证 ---

// SignPSS 使用 PSS 签名（推荐）
func SignPSS(message []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.Sum256(message)
	return rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], nil)
}

// VerifyPSS 验证 PSS 签名
func VerifyPSS(message, signature []byte, publicKey *rsa.PublicKey) error {
	hash := sha256.Sum256(message)
	return rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signature, nil)
}

// SignPKCS1v15 使用 PKCS1v15 签名
func SignPKCS1v15(message []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := sha256.Sum256(message)
	return rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
}

// VerifyPKCS1v15 验证 PKCS1v15 签名
func VerifyPKCS1v15(message, signature []byte, publicKey *rsa.PublicKey) error {
	hash := sha256.Sum256(message)
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
}

// SignString 签名字符串，返回 Base64
func SignString(message string, privateKeyPEM string) (string, error) {
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
func VerifyString(message, signature string, publicKeyPEM string) error {
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
	return EncryptOAEP(plaintext, kp.PublicKey)
}

// Decrypt 使用 KeyPair 解密
func (kp *KeyPair) Decrypt(ciphertext []byte) ([]byte, error) {
	return DecryptOAEP(ciphertext, kp.PrivateKey)
}

// Sign 使用 KeyPair 签名
func (kp *KeyPair) Sign(message []byte) ([]byte, error) {
	return SignPSS(message, kp.PrivateKey)
}

// Verify 使用 KeyPair 验签
func (kp *KeyPair) Verify(message, signature []byte) error {
	return VerifyPSS(message, signature, kp.PublicKey)
}
