// Package sign provides digital signature utilities.
//
// Supports HMAC, RSA, and ECDSA signatures.
//
// HMAC signing:
//
//	signature := sign.HMAC(message, secret)
//	valid := sign.VerifyHMAC(message, signature, secret)
//
// With algorithm options:
//
//	signature := sign.HMAC(message, secret, sign.WithAlgorithm(sign.SHA256))
package sign
