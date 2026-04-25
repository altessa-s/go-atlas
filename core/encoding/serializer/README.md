# serializer

```go
import "github.com/altessa-s/go-atlas/core/encoding/serializer"
```

Package `serializer` defines a format-agnostic `Serializer` interface for encoding and decoding Go values. It is used by cache, uniq, and
other packages that need pluggable marshaling without a hard dependency on a specific encoding format.

Implementations must be stateless and safe for concurrent use. The zero value of `JSON` is ready to use without any initialization.

## Implementations

| Type   | Backend          | Notes                       |
|--------|------------------|-----------------------------|
| `JSON` | `encoding/json`  | Zero value ready to use     |
