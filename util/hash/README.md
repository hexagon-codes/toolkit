中文 | [English](README.en.md)

# Hash 哈希工具

提供 SHA-256、SHA-512 内容摘要与 bcrypt 密码哈希。

```go
digest := hash.SHA256("data")

passwordHash, err := hash.BcryptHash(password)
if err != nil {
    return err
}
if !hash.BcryptCheck(password, passwordHash) {
    return errors.New("密码不匹配")
}
```

SHA-256 和 SHA-512 不应直接用于密码存储，也不提供消息认证；密码使用 bcrypt，
消息认证使用 HMAC，数字签名使用签名组件。
