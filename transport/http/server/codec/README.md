# codec

```go
import "github.com/altessa-s/go-atlas/transport/http/server/codec"
```

Package `codec` provides HTTP body encoding/decoding with content negotiation. A thread-safe `Registry` maps MIME types to codec
implementations. The `DefaultRegistry` includes JSON and XML codecs (with alternate MIME types) registered at init time. Content
negotiation is performed by `Registry.Negotiate`, which parses the HTTP Accept header and selects the best matching encoder.

## Key types

| Type / Interface    | Description                                                                            |
|---------------------|----------------------------------------------------------------------------------------|
| `Encoder`           | Interface: `Encode(data) ([]byte, error)` + `ContentType() string`                    |
| `Decoder`           | Interface: `Decode(data, out) error` + `ContentType() string`                          |
| `Codec`             | Combines `Encoder` and `Decoder` for a single MIME type                                |
| `StreamingEncoder`  | Interface: `EncodeStream(w, data) error` — writes directly to `io.Writer`              |
| `StreamingCodec`    | Combines `StreamingEncoder` and `Decoder` for large payloads                           |
| `Registry`          | Thread-safe MIME-to-codec mapping with `Register`, `Get`, and `Negotiate` methods      |

## Functions

| Function          | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `DefaultRegistry` | Returns the global registry pre-loaded with JSON and XML codecs                          |

## Subpackages

| Package                              | Description                                                        |
|--------------------------------------|--------------------------------------------------------------------|
| [providers/json](./providers/json)   | JSON codec backed by `encoding/json`                               |
| [providers/xml](./providers/xml)     | XML codec backed by `encoding/xml`                                 |
