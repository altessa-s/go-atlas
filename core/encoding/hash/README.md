# hash

```go
import "github.com/altessa-s/go-atlas/core/encoding/hash"
```

Package `hash` provides stdlib-only helpers for producing deterministic, hex-encoded SHA-256 digest strings. All stateless functions are
safe for concurrent use and return a 64-character lowercase hex string suitable for cache keys and deduplication.

For streaming or incremental hashing, use `NewSHA256Hasher`. It implements `Hasher` (`io.Writer` + `SumHex()`) and integrates directly
with `io.MultiWriter`.

Intended for cache keys, content addressing, and deduplication tokens — **not** for password hashing (use bcrypt/scrypt/argon2 instead).

## Key types

| Type            | Description                                                             |
|-----------------|-------------------------------------------------------------------------|
| `Hasher`        | Interface: `io.Writer` + `SumHex() string`                              |
| `SHA256Hasher`  | SHA-256 implementation of `Hasher`; construct with `NewSHA256Hasher`    |

## Functions

| Function                   | Description                                                  |
|----------------------------|--------------------------------------------------------------|
| `SHA256HexBytes`           | Digest of a `[]byte`                                         |
| `SHA256HexString`          | Digest of a `string` (zero-copy)                             |
| `SHA256HexWithPrefix`      | `prefix + hex(SHA-256(s))` for namespaced keys               |
| `SHA256HexWithSalt`        | Digest of `salt \|\| data` (`[]byte` variant)                |
| `SHA256HexStringWithSalt`  | Digest of `salt \|\| s` (`string` variant, zero-copy)        |
| `NewSHA256Hasher`          | Returns a `*SHA256Hasher` ready for incremental writes       |
| `HexSum`                   | Hex-encodes the current digest of any `hash.Hash`            |

## Usage

### Streaming with io.MultiWriter

```go
var dst io.Writer // e.g. a file or buffer
h := hash.NewSHA256Hasher()
if _, err := io.Copy(io.MultiWriter(dst, h), src); err != nil {
    return err
}
checksum := h.SumHex()
```

### Algorithm-agnostic hex encoding

```go
h := sha256.New()
h.Write(data)
digest := hash.HexSum(h)
```
