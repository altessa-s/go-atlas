# serializer

```go
import "github.com/altessa-s/go-atlas/core/encoding/serializer"
```

Package `serializer` defines a format-agnostic `Serializer` interface for encoding and decoding Go values. It is used by cache, uniq, and other 
packages that need pluggable marshaling.

## Interface

```go
type Serializer interface {
    Serialize(data any) ([]byte, error)
    Deserialize(d []byte, out any) error
}
```

Implementations must be stateless and safe for concurrent use.

## Implementations

| Type   | Backend          | Notes                       |
|--------|------------------|-----------------------------|
| `JSON` | `encoding/json`  | Zero value ready to use     |

## Usage

```go
s := &serializer.JSON{}

data, err := s.Serialize(myStruct)
// ...

var result MyStruct
err = s.Deserialize(data, &result)
```
