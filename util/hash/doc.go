// Package hash 提供 SHA-256、SHA-512 和 bcrypt 能力。
//
// SHA 系列适合内容摘要，不适合直接存储密码；密码应使用 BcryptHash，验证时
// 使用 BcryptCheck。需要消息认证或签名时应使用专门的 HMAC 或签名组件。
package hash
