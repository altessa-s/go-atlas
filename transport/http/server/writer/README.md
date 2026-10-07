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
before encoding, so values accepted only on the write path (passwords, one-time tokens) are not echoed on the read path. This mirrors the
gRPC `fieldbehavior` interceptor for the HTTP transport.

Sanitization is **on by default**:

- The handler's message is never mutated: a strict-mode pass detects populated INPUT_ONLY fields without touching it, and they are cleared
  on a `proto.Clone` copy that gets encoded. The copy is made only when such a field is present.
- Nested, repeated and map message fields and oneofs are covered. OUTPUT_ONLY and other behaviors are left in the response.
- Traversal is bounded by `WithResponseSanitizationMaxDepth` (default `fieldbehavior.DefaultMaxDepth`, 32). When a response nests deeper,
  or sanitization fails otherwise, the client receives a 500 without the payload, the cause is logged, and the call returns an error
  wrapping `ErrResponseSanitization`.

Only the message passed as the response is inspected. Not sanitized:

- the contents of `google.protobuf.Any` fields (opaque bytes), unless the `Any` field itself is INPUT_ONLY;
- proto messages inside slices, maps or caller-defined envelopes, and payloads added by a custom `Builder`;
- error bodies written by `WriteError`.

Non-proto responses pass through unchanged. Disable sanitization with `WithResponseSanitizationDisabled()` when a service has an external
reason to emit INPUT_ONLY fields on the read path. See [`domain/proto/fieldbehavior`](../../../../domain/proto/fieldbehavior/README.md) for
the underlying strip semantics.
