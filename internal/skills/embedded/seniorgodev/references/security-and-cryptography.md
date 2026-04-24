# Security and Cryptography

## Secure Random

```go
import "crypto/rand"

// Random bytes
buf := make([]byte, 32)
rand.Read(buf)

// Random text (Go 1.24+)
text := rand.Text()
```

## Hashing

```go
import "crypto/sha256"

// One-shot
sum := sha256.Sum256(data)
```

## Post-Quantum Cryptography (Go 1.24+)

```go
import "crypto/mlkem"

pub, priv, _ := mlkem.GenerateKey768()
ciphertext, sharedKey := pub.Encapsulate()
decryptedKey := priv.Decapsulate(ciphertext)
```

## TLS Configuration

```go
config := &tls.Config{
    MinVersion: tls.VersionTLS13,
    // Post-quantum enabled by default in Go 1.26
}
```

## HPKE (Go 1.26+)

```go
import "crypto/hpke"

suite := hpke.NewSuite(hpke.KEM_X25519, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES128GCM)
sender, enc := suite.NewSender(recipientPub, info)
ciphertext := sender.Seal(plaintext, aad)
```
