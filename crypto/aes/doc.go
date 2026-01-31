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
