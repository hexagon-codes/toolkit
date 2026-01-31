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
