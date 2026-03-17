// Package rsa 提供 RSA 加密和解密工具
//
// 支持密钥生成、加密/解密和签名操作。
//
// 密钥生成:
//
//	privateKey, publicKey, err := rsa.GenerateKeyPair(2048)
//
// 加密解密:
//
//	encrypted, err := rsa.Encrypt(plaintext, publicKey)
//	decrypted, err := rsa.Decrypt(encrypted, privateKey)
//
// 签名验签:
//
//	signature, err := rsa.Sign(message, privateKey)
//	valid := rsa.Verify(message, signature, publicKey)
//
// --- English ---
//
// Package rsa provides RSA encryption and decryption utilities.
//
// Supports key generation, encryption/decryption, and signing.
//
// Key generation:
//
//	privateKey, publicKey, err := rsa.GenerateKeyPair(2048)
//
// Encryption:
//
//	encrypted, err := rsa.Encrypt(plaintext, publicKey)
//	decrypted, err := rsa.Decrypt(encrypted, privateKey)
//
// Signing:
//
//	signature, err := rsa.Sign(message, privateKey)
//	valid := rsa.Verify(message, signature, publicKey)
package rsa
