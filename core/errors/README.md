# errors

```go
import "github.com/altessa-s/go-atlas/core/errors"
```

Package `errors` provides error classification, wrapping, and network error inspection utilities. All functions are nil-safe and preserve the full error
chain for `errors.Is` / `errors.As` unwrapping across any number of nested wrappers.

## Wrapping

| Function                   | Message format                                          |
|----------------------------|---------------------------------------------------------|
| `Wrap`                     | `<msg>: <err>`                                          |
| `Wrapf`                    | `<format args>: <err>`                                  |
| `WrapOperation`            | `failed to <operation>: <err>`                          |
| `WrapOperationWithContext` | `failed to <operation> on <context>: <err>`             |
| `WrapField`                | `field '<name>': <err>`                                 |
| `JoinWrap`                 | `<sentinel>: <cause>` — both stay matchable via `errors.Is` |

## Construction

| Function   | Message format                                    |
|------------|---------------------------------------------------|
| `Required` | `<dependency> is required for <context>`          |
| `Provider` | `failed to create <type> provider: <err>`         |

## Canonical sentinels

Transport-independent classification for domain and storage layers: wrap a sentinel (`fmt.Errorf("%w: ...", errors.ErrNotFound)`) and test it
with `errors.Is`. The gRPC errstatus interceptor maps them with `WithCanonicalRules`.

| Symbol                         | Meaning                                                                                     |
|--------------------------------|---------------------------------------------------------------------------------------------|
| `ErrNotFound`                  | The requested entity does not exist                                                         |
| `ErrAlreadyExists`             | An entity with the same identity already exists                                             |
| `ErrConflict`                  | Optimistic-concurrency conflict (version or etag mismatch); re-read and retry               |
| `ErrInvalidArgument`           | Semantically invalid request; its text is published to clients, so keep it client-safe      |
| `ErrFailedPrecondition`        | The system is not in the state the operation requires                                       |
| `ErrUnavailable`               | Transient failure of a dependency; retry later                                              |
| `ErrBusy`                      | The same work is in progress elsewhere; retry later                                         |
| `FieldError{Field, Reason, Err}` | An `ErrInvalidArgument` naming the offending field; optional `Err` cause stays in the chain; `NewFieldError(field, reason)` |
| `MapFieldPath(err, mapPath)`   | Re-roots the wrapped `FieldError` under another path, keeping the chain and the text        |

## Context checks

| Function                               | Matches                                       |
|----------------------------------------|-----------------------------------------------|
| `IsContextCanceled`                    | `context.Canceled`                             |
| `IsContextDeadlineExceeded`            | `context.DeadlineExceeded`                     |
| `IsContextCanceledOrDeadlineExceeded`  | Either of the above                            |

## Network checks

| Function                       | Detects                                          |
|--------------------------------|--------------------------------------------------|
| `IsNetworkError`               | Any `net.Error`                                  |
| `IsRequestTimeoutError`        | `net.Error` or `*url.Error` with `Timeout()`     |
| `IsURLError`                   | Any `*url.Error`                                 |
| `IsConnectionRefused`          | `syscall.ECONNREFUSED` through URL/net layers    |
| `IsResourceRedirects`          | "stopped after N redirects"                      |
| `IsUnsupportedProtocolScheme`  | "unsupported protocol scheme"                    |
| `IsCertUnknownAuthority`       | `x509.UnknownAuthorityError` inside `*url.Error` |
