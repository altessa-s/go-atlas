# errors

```go
import "github.com/altessa-s/go-atlas/core/errors"
```

Package `errors` provides error classification, wrapping, and network error inspection utilities. All functions are nil-safe and preserve the 
error chain for `errors.Is` / `errors.As`.

## Wrapping

| Function                   | Message format                                          |
|----------------------------|---------------------------------------------------------|
| `Wrap`                     | `<msg>: <err>`                                          |
| `Wrapf`                    | `<format args>: <err>`                                  |
| `WrapOperation`            | `failed to <operation>: <err>`                          |
| `WrapOperationWithContext` | `failed to <operation> on <context>: <err>`             |
| `WrapField`                | `field '<name>': <err>`                                 |

## Construction

| Function   | Message format                                    |
|------------|---------------------------------------------------|
| `Required` | `<dependency> is required for <context>`          |
| `Provider` | `failed to create <type> provider: <err>`         |

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

## Generic type assertion

```go
if appErr, ok := errors.AsType[*AppError](err); ok {
    // use appErr directly
}
```

`AsType[E]` is a generic alternative to `errors.As` that returns the matched value directly. On Go 1.26+ it delegates to `errors.AsType` from the standard library.
