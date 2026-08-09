// Package sign 提供基于 SHA-2 的 HMAC 签名、时间戳签名和 API 请求签名工具。
//
// HMAC 签名：
//
//	signature := sign.HMACSHA256(message, secret)
//	valid := sign.VerifyHMACSHA256(message, secret, signature)
//
// 指定 SHA-2 算法：
//
//	signature := sign.HMAC(message, secret, sign.SHA512)
//	valid := sign.VerifyHMAC(message, secret, signature, sign.SHA512)
package sign
