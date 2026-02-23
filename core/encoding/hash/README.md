# hash

```go
import "github.com/altessa-s/go-atlas/core/encoding/hash"
```

Package `hash` provides stdlib-only helpers for producing deterministic, hex-encoded SHA-256 digest strings. All functions are stateless, safe for 
concurrent use, and return a 64-character lowercase hex string.

Intended for cache keys, content addressing, and deduplication tokens — **not** for password hashing (use bcrypt/scrypt/argon2 instead).

## Functions

| Function                   | Description                                                  |
|----------------------------|--------------------------------------------------------------|
| `SHA256HexBytes`           | Digest of a `[]byte`                                         |
| `SHA256HexString`          | Digest of a `string` (zero-copy)                             |
| `SHA256HexWithPrefix`      | `prefix + hex(SHA-256(s))` for namespaced keys               |
| `SHA256HexWithSalt`        | Digest of `salt \|\| data` (`[]byte` variant)                |
| `SHA256HexStringWithSalt`  | Digest of `salt \|\| s` (`string` variant, zero-copy)        |

## Usage

```go
// Simple digest
digest := hash.SHA256HexString("hello world")

// Namespaced cache key
key := hash.SHA256HexWithPrefix("cache:", token)

// Salted digest for domain separation
h := hash.SHA256HexStringWithSalt(userID, "session-v1")
```
