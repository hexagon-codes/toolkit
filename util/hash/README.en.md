[中文](README.md) | English

# Hash utilities

This package provides SHA-256 and SHA-512 content digests plus bcrypt password
hashing.

```go
digest := hash.SHA256("data")

passwordHash, err := hash.BcryptHash(password)
if err != nil {
    return err
}
if !hash.BcryptCheck(password, passwordHash) {
    return errors.New("password mismatch")
}
```

Do not use SHA-256 or SHA-512 directly for password storage or message
authentication. Use bcrypt for passwords, HMAC for authentication, and a
signature component for digital signatures.
