# base64

```go
import "github.com/altessa-s/go-atlas/security/secrets/codec/keys/base64"
```

Package `base64` provides a `KeyDecoder` implementation using base64 URL-safe encoding (`RawURLEncoding`)
for safe use in URL paths of secret storage systems.

## Usage

```go
dec := base64.NewKeyDecoder()
encoded := dec.Encode("my/secret/key")  // URL-safe base64
original, err := dec.Decode(encoded)
```
