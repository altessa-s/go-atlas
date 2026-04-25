# writer

```go
import "github.com/altessa-s/go-atlas/transport/http/server/writer"
```

Package `writer` provides HTTP response writing with content negotiation and request body decoding. `Writer` selects a `codec.Encoder`
by negotiating the Accept header against the `codec.Registry`, then encodes the response through a configurable `Builder` that wraps
data in a `Response` envelope. `ReadWriter` combines read and write operations into a per-request helper that is pooled for efficiency.

## Key types

| Type / Interface | Description                                                                            |
|------------------|----------------------------------------------------------------------------------------|
| `Writer`         | Core response writer with content negotiation via Accept header                        |
| `ReadWriter`     | Per-request pooled helper combining request body decoding and response writing         |
| `Builder`        | Interface for wrapping response data in an envelope (default: `Default`)               |
| `Response`       | Standard response envelope with data, error, and metadata fields                       |
| `Coder`          | Interface for errors that provide a machine-readable error code                        |
| `Messager`       | Interface for errors that provide a custom user-facing message                         |
| `HTTPStatuser`   | Interface for errors that specify their HTTP status code                                |
