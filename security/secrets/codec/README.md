# codec

```go
import "github.com/altessa-s/go-atlas/security/secrets/codec"
```

Package `codec` defines interfaces for encoding and decoding secret keys and values.
Implementations allow pluggable serialization formats for secret storage systems.

## Key types

| Type              | Description                                         |
|-------------------|-----------------------------------------------------|
| `KeyDecoder`      | Encode/decode secret keys (e.g., base64 URL-safe)   |
| `ValueDecoder[T]` | Encode/decode secret values (e.g., JSON)            |

## Subpackages

| Package                          | Description                        |
|----------------------------------|------------------------------------|
| [keys/base64](./keys/base64)    | Base64 URL-safe key encoding       |
| [values/json](./values/json)    | JSON value serialization           |
