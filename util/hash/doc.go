// Package hash 提供哈希工具函数
//
// 包括 MD5、SHA256 和 HMAC 实现。
//
// 基本用法:
//
//	md5 := hash.MD5("hello")
//	sha256 := hash.SHA256("hello")
//	hmac := hash.HMAC("message", "secret")
//
// 文件哈希:
//
//	md5, err := hash.MD5File("/path/to/file")
//
// --- English ---
//
// Package hash provides hash utilities.
//
// Includes MD5, SHA256, and HMAC implementations.
//
// Basic usage:
//
//	md5 := hash.MD5("hello")
//	sha256 := hash.SHA256("hello")
//	hmac := hash.HMAC("message", "secret")
//
// For files:
//
//	md5, err := hash.MD5File("/path/to/file")
package hash
