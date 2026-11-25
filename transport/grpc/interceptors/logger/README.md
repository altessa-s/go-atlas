# logger

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
```

Package `logger` provides gRPC interceptors for request and response logging. Captures method calls, timing, gRPC status codes, errors, request/response
payloads, request IDs, trace correlation, and client IP with structured logging support via a pluggable `Logger` interface.

## Key types

| Type / Interface | Description                                                                     |
|------------------|---------------------------------------------------------------------------------|
| `Logger`         | Interface: `Log(ctx, msg, grpcCode, fields)` for structured call completion events |
| `LoggerFunc`     | Function adapter for the `Logger` interface                                     |
| `Slog`           | Ready-made implementation that maps gRPC codes to `slog.Level`                  |
| `PayloadRedactor`| `func(proto.Message) any` that redacts payloads before they are logged          |
| `DebugRedact`    | `PayloadRedactor` clearing `debug_redact` fields in a copy (cached per descriptor) |

## Options

| Option                      | Default            | Description                                       |
|-----------------------------|--------------------|---------------------------------------------------|
| `WithTimeFormat`            | RFC3339            | Timestamp formatting style                        |
| `WithLogResponse`           | false              | Include response payloads in log output            |
| `WithLogRequest`            | false              | Include request payloads in log output             |
| `WithIgnoreMethods`         | --                 | Methods to skip logging                            |
| `WithIgnorePatterns`        | reflection, health | Regex patterns for methods to skip, matched against the lowercased method |
| `WithLogGrpcResponseCodes`  | `DefaultLogCodes`  | gRPC response codes to always log                  |
| `WithIgnoreGrpcResponseCodes` | --               | gRPC response codes to never log (takes precedence) |
| `WithContextLogger`         | nil                | Store enriched logger in context for handlers       |
| `WithPayloadRedactor`       | nil                | `PayloadRedactor` applied to logged request/response payloads |
| `WithDebugRedact`           | off                | Clear `debug_redact` fields from logged payloads, before the payload redactor |
| `WithMaxMethodLength`       | 1024               | Cap, in bytes, of the logged service and method names |
| `WithMaxUserAgentLength`    | 512                | Cap, in bytes, of the logged User-Agent             |
| `WithMaxMessageLength`      | 1024               | Cap, in bytes, of the logged status message and underlying error text |

## Method, User-Agent and messages are sanitized

The method names, the User-Agent, the status message, the `ErrorInfo` reason and the error text are peer-controlled or echo request values. Each is
truncated to its cap and marked with `TruncationSuffix` (`...`), and its control characters (CR, LF, the rest of C0, DEL and C1) are removed, so
one call can neither forge a log line nor bloat the record. Error text and status messages also have URL user information removed before truncation.
With server error conversion enabled, `grpc.code` and `grpc.message` describe the finalized status, and `error` carries the cause from before
conversion. Unary and streaming completion records measure elapsed time from the start of their own RPC.

## Redacting payloads

With `WithLogRequest` / `WithLogResponse` the payload protos are logged. `WithDebugRedact` clears the values of the fields marked with the standard
`debug_redact` option (`string api_key = 2 [debug_redact = true];`) at any depth — nested messages, lists, map values, recursive types — in a copy;
the message the handler receives or returns is never changed, and a message type that cannot carry such a field is logged without a copy. Whether a
type can carry one is computed once per message descriptor. A `google.protobuf.Any` may hold anything, so a type containing one is always walked: a
payload whose type (resolved through `protoregistry.GlobalTypes`) can carry such a field is unpacked, redacted and repacked, and a payload that cannot
be checked (unresolvable type URL, malformed bytes — every resolved payload is decoded, also of types without secrets) fails closed — the Any
becomes an empty `google.protobuf.Empty`. The unpacking of one logged message is bounded: 16 nested Any layers and 1 MiB of decoded payload bytes in
total; payloads beyond the bounds get the same placeholder, so crafted nesting cannot make logging expensive. A `WithPayloadRedactor` redactor
then receives the redacted copy. The server factory
turns it on with `interceptors.logger.debugRedact` (default `true`).

```go
logger.ServerInterceptor(logger.Slog(l), logger.WithLogRequest(), logger.WithDebugRedact())
```

## Diagnostics

Completion records include the gRPC code, a valid ErrorInfo reason when available, and a recovery category for panics. Use `WithLogClientDetails(true)` for peer IP, real IP, and User-Agent, and `WithLogRawErrors(true)` for error text and panic diagnostics. The slog adapter emits dotted keys such as `client.peer_ip` as literal keys for masking.
