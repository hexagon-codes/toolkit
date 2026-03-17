// Package aes 提供 AES 加密和解密工具
//
// 支持 CBC 和 GCM 模式，使用 PKCS7 填充。
//
// 基本用法:
//
//	encrypted, err := aes.Encrypt(plaintext, key)
//	decrypted, err := aes.Decrypt(encrypted, key)
//
// 带 IV 的用法:
//
//	encrypted, iv, err := aes.EncryptCBC(plaintext, key)
//	decrypted, err := aes.DecryptCBC(encrypted, key, iv)
//
// GCM 模式（推荐）:
//
//	encrypted, nonce, err := aes.EncryptGCM(plaintext, key)
//	decrypted, err := aes.DecryptGCM(encrypted, key, nonce)
//
// --- English ---
//
// Package aes provides AES encryption and decryption utilities.
//
// Supports CBC and GCM modes with PKCS7 padding.
//
// Basic usage:
//
//	encrypted, err := aes.Encrypt(plaintext, key)
//	decrypted, err := aes.Decrypt(encrypted, key)
//
// With IV:
//
//	encrypted, iv, err := aes.EncryptCBC(plaintext, key)
//	decrypted, err := aes.DecryptCBC(encrypted, key, iv)
//
// GCM mode (recommended):
//
//	encrypted, nonce, err := aes.EncryptGCM(plaintext, key)
//	decrypted, err := aes.DecryptGCM(encrypted, key, nonce)
package aes
