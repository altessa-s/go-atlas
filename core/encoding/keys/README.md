# keys

```go
import "github.com/altessa-s/go-atlas/core/encoding/keys"
```

Unambiguous keys from ordered tuples of parts. Joining parts with a separator lets two tuples share a key (`"a:b" + "c"` and `"a" + "b:c"`); both
helpers frame every part with its length instead. No external dependencies.

| Function           | Result                                                                                                       |
|--------------------|--------------------------------------------------------------------------------------------------------------|
| `Compose(parts...)` | For every part: its byte length in decimal, `:`, its bytes. `("a:b", "c")` → `3:a:b1:c`, `("a", "b:c")` → `1:a3:b:c`, no parts → `""` |
| `Digest(parts...)`  | SHA-256 over every part prefixed with its length as an 8-byte big-endian integer (32 bytes)                 |

Both layouts are frozen: callers persist the keys. `data/idempotency.ComposeKey` is `"idk:v2:" + Compose(parts...)`, and the idempotency
transports store `Digest` values as request fingerprints.

```go
k := keys.Compose("transport", service, idempotencyKey)
fp := keys.Digest([]byte(method), body)
cacheKey := hex.EncodeToString(keys.Digest([]byte(filter), []byte(orderBy)))
```
