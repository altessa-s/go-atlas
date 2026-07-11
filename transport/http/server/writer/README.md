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

## Response sanitization

When the response value is a `proto.Message`, `Write` and `WriteStream` strip fields annotated `google.api.field_behavior = INPUT_ONLY`
before encoding, so write-path secrets (passwords, one-time tokens) never leak back to clients on the read path. This mirrors the gRPC
`fieldbehavior` interceptor for the HTTP transport.

Sanitization is **on by default** and secure-by-default:

- The handler's message is never mutated — a strict-mode detection pass leaves the original untouched, and the INPUT_ONLY fields are
  cleared on a `proto.Clone` copy that gets encoded.
- The clone is allocated only when a populated INPUT_ONLY field is actually present. A proto response with nothing to strip — the common
  case — incurs a single read-only traversal and no copy.
- Non-proto responses (maps, structs, slices) pass through unchanged.

Disable it with `WithResponseSanitizationDisabled()` when a service has an external reason to emit INPUT_ONLY fields on the read path.
See [`domain/proto/fieldbehavior`](../../../../domain/proto/fieldbehavior/README.md) for the underlying strip semantics.
