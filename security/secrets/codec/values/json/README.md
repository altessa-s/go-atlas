# json

```go
import "github.com/altessa-s/go-atlas/security/secrets/codec/values/json"
```

Package `json` provides a generic `ValueDecoder[T]` implementation using JSON encoding for serializing
and deserializing secret values of any type `T`.

## Usage

```go
dec := json.NewValueDecoder[MyConfig]()
data, err := dec.Encode(config)
restored, err := dec.Decode(data)
```
