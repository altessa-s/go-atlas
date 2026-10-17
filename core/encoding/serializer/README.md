# serializer

```go
import "github.com/altessa-s/go-atlas/core/encoding/serializer"
```

Package `serializer` defines a format-agnostic `Serializer` interface for encoding and decoding Go values. It is used by `data/cache`,
`data/idempotency`, `data/saga`, and other packages that need pluggable marshaling without a hard dependency on a specific encoding format.

The interface has two methods: `Serialize(data any) ([]byte, error)` and `Deserialize(d []byte, out any) error`, where `out` must be a non-nil pointer.
Implementations must be stateless and safe for concurrent use. The zero value of `JSON` is ready to use without any initialization.

## Implementations

| Type   | Backend          | Notes                       |
|--------|------------------|-----------------------------|
| `JSON` | `encoding/json`  | Zero value ready to use     |
