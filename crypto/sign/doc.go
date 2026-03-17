// Package sign 提供数字签名工具
//
// 支持 HMAC、RSA 和 ECDSA 签名算法。
//
// HMAC 签名:
//
//	signature := sign.HMAC(message, secret)
//	valid := sign.VerifyHMAC(message, signature, secret)
//
// 指定算法:
//
//	signature := sign.HMAC(message, secret, sign.WithAlgorithm(sign.SHA256))
//
// --- English ---
//
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
