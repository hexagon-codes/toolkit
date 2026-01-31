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
