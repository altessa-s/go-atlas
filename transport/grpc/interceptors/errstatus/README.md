# errstatus

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
```

Package `errstatus` provides gRPC interceptors for consistent error-to-status and status-to-error conversion. Server-side: `ErrorConverter` mappings
with optional `Finalizer` and a per-converter status cache. Client-side: `StatusConverter` for translating gRPC statuses back to application errors.

## Key types

| Type / Interface  | Description                                                                     |
|-------------------|---------------------------------------------------------------------------------|
| `ErrorConverter`  | Server-side: Matcher + Convert function for transforming errors to gRPC status; `Static` marks a status that does not depend on the error and may be cached |
| `ErrorRule`       | Server-side: sentinel → code, message, `ErrorInfo.Reason`; error text private unless `Expose` |
| `StatusConverter` | Client-side: Matcher + Convert function for translating statuses to errors      |
| `Finalizer`       | Post-processing function for enriching converted errors (e.g., ErrorInfo)       |
| `StatusError`     | Interface for service-level custom error conversion                             |

## Server options

| Option                     | Default               | Description                                       |
|----------------------------|-----------------------|---------------------------------------------------|
| `WithErrorConverters`      | --                    | Custom error-to-status converters (priority order) |
| `WithErrorMapping`         | --                    | Simple error-to-code mapping via `errors.Is`      |
| `WithErrorRules`           | --                    | `ErrorRule` mappings: code, `ErrorInfo.Reason`, message hidden unless `Expose` |
| `WithErrorTypeMapping`     | --                    | Type-safe converter using `errors.As` with generics |
| `WithFieldErrorType`       | --                    | Domain field error type `T` → `InvalidArgument` with one BadRequest violation (`BadRequest`) |
| `WithCanonicalRules`       | --                    | Converters for the canonical [`go-atlas-core/errors`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors) sentinels and `FieldError` (see below) |
| `WithCanonicalAPIRules`    | --                    | `WithCanonicalRules` with `WithFieldPathMapper(CamelPath)`: lowerCamelCase violation paths |
| `WithFieldPathMapper`      | nil (as written)      | Maps a canonical `FieldError.Field` to the API path its violation names (e.g. `CamelPath`) |
| `WithFinalizer`            | nil (none applied)    | Post-processing for converted errors              |
| `WithDomain`               | --                    | `ErrorInfo.Domain`; runs after the `WithFinalizer` finalizer, which it does not replace; no effect without one |
| `WithCacheSize`            | 1000                  | Number of converters, in registration order, whose static status is cached |
| `WithCacheDisabled`        | false                 | Completely disable error conversion caching        |
| `WithLogger`               | discard               | Structured logger                                  |

Server logging and tracing can read the error received by `errstatus` before `StatusErrorConvert`, registered converters and the finalizer run.
The completion log's `error` and the span's `exception.message` omit URL user information. The client receives the finalized status and details.
A service converter that logs its input can therefore emit a second record for the same failure.

## Errors that already carry a status

An error that already carries a gRPC status skips the converters. A status the handler returned directly (`status.Error(codes.NotFound, "user
not found")`, or an `interceptors.Error`) with a code other than `Internal`, `Unknown` or `DataLoss` reaches the client unchanged. Otherwise the
status is rewritten and the original error is logged at warn level:

| Returned error                                                | Client message                              | Details                  |
|---------------------------------------------------------------|---------------------------------------------|--------------------------|
| direct status, client-facing code                             | as returned                                 | kept, except `DebugInfo` |
| `Internal`, direct or wrapped                                 | `Internal Server Error` (`MessageInternal`) | server-fault allowlist   |
| `Unknown` / `DataLoss`, direct or wrapped                     | the code's name (`Unknown`, `DataLoss`)     | server-fault allowlist   |
| client-facing code wrapped by `fmt.Errorf("...: %w", err)`    | the wrapped status's own message            | client-facing allowlist  |

A wrapped status is rewritten because `status.FromError` reports the whole `err.Error()` chain as its message; only the wrapper text is discarded.
Details outside the allowlist are dropped and their type names are logged at debug level:

| Allowlist     | Detail types                                                                                                                       |
|---------------|------------------------------------------------------------------------------------------------------------------------------------|
| server-fault  | `RetryInfo`, `RequestInfo`, `LocalizedMessage`, `Help`                                                                             |
| client-facing | the server-fault types plus `BadRequest` (`google.rpc` and `io.altessa.badrequest.v1`), `PreconditionFailure`, `QuotaFailure`, `ResourceInfo`, `ErrorInfo` |

`errdetails.DebugInfo` is in neither list and is never forwarded: it is also removed from a direct status with a client-facing code, whose other
details, custom types included, pass through. That status is rebuilt only when it carries a `DebugInfo`; without one it is returned as is, and
the check allocates nothing for a status without details. The finalizer runs afterwards, so `DefaultFinalizer` still adds its `ErrorInfo` and `RequestInfo`.

## Error rules

The status a converter returns (`WithErrorConverters`, `WithErrorMapping`, `WithErrorTypeMapping`, `WithErrorRules`) is treated like a status a
handler returned directly: for `Internal`, `Unknown` and `DataLoss` the client gets the fixed message and only the server-fault details, whatever
message or details the converter set, and `DebugInfo` is removed from any status. This happens before the status is cached.

The matchers run for every error, in registration order, and the first match wins. Only the status is cached, and only for a converter whose
status does not depend on the error: a rule or mapping with a fixed message, or an `ErrorConverter` with `Static: true`. The entry belongs to the
converter that matched, never to a sentinel or the text of the error, so an error wrapping several targets cannot make one target answer for
another. A status that takes anything from the error (`WithErrorMapping` with an empty message, `WithErrorTypeMapping`, an exposed rule, any
`ErrorConverter` without `Static`) is converted on every call.

`WithErrorMapping` publishes `err.Error()` when its message is empty and the code is client-facing. `WithErrorRules` keeps the error text private instead: a rule without a `Message`
sends the code's name (`"NotFound"`) unless `Expose` is set. A non-empty `Reason` is attached as `ErrorInfo.Reason`; the finalizer keeps it and fills in
the domain.

```go
errstatus.WithErrorRules(
    errstatus.ErrorRule{Target: ErrQuota, Code: codes.ResourceExhausted, Reason: "QUOTA_EXHAUSTED"},
    errstatus.ErrorRule{Target: ErrBadCursor, Code: codes.InvalidArgument, Expose: true},
)
```

## Field errors

`WithFieldErrorType[T](func(T) (path, reason string))` turns a domain field error into the status of `BadRequest(path, reason)`: message
`"<path>: <reason>"` and one `io.altessa.badrequest.v1.BadRequest` violation (field path and message, no machine code), so a handler returns its
domain error as is. The function reports the API field path, typically through a `PathMapper` such as `CamelPath`. The converter matches with
`errors.As` at any depth, takes its place in registration order (a rule registered before it wins for an error that wraps both) and is never
cached, since the status depends on the field. The default finalizer adds the `INVALID_ARGUMENT` reason.

| Function                      | Description                                                                                               |
|-------------------------------|-----------------------------------------------------------------------------------------------------------|
| `BadRequest(path, reason)`    | `InvalidArgument` status with message `"<path>: <reason>"` and one violation (`field_path`, `message`)     |
| `FieldViolation(path, reason)` | `BadRequest(path, reason).Err()`, for a handler that reports a violation itself                          |
| `PathMapper`                  | `func(field string) string`: a domain field path to the API path a violation names                        |
| `CamelPath(path)`             | Every dot-separated segment in lowerCamelCase (`attrs.law_code` → `attrs.lawCode`); idempotent            |
| `Prefixed(prefix)`            | `PathMapper` that camel-cases and puts the path under `prefix` (`provider` + `base_url` → `provider.baseUrl`) |
| `Rooted(err, prefix)`         | [`go-atlas-core/errors.MapFieldPath(err, Prefixed(prefix))`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors): re-roots a domain `FieldError` under its request field |

```go
errstatus.ServerInterceptor(
    errstatus.WithErrorRules(errstatus.ErrorRule{Target: ErrNotFound, Code: codes.NotFound, Message: "not found"}),
    errstatus.WithFieldErrorType(func(e *FieldError) (string, string) { return errstatus.CamelPath(e.Field), e.Reason }),
)
```

## Canonical rules

`WithCanonicalRules()` maps the canonical domain sentinels of
[`go-atlas-core/errors`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors/README.md), in this order (first match wins):

| Error                   | Code                 | Message                                     | `ErrorInfo.Reason`      |
|-------------------------|----------------------|---------------------------------------------|-------------------------|
| `ErrNotFound`           | `NotFound`           | `not found`                                 | `NOT_FOUND`             |
| `ErrAlreadyExists`      | `AlreadyExists`      | `already exists`                            | `ALREADY_EXISTS`        |
| `ErrConflict`           | `Aborted`            | `changed concurrently, re-read and retry`   | `VERSION_CONFLICT`      |
| `*FieldError`           | `InvalidArgument`    | `<path>: <reason>` + one BadRequest violation | `INVALID_ARGUMENT`    |
| `ErrInvalidArgument`    | `InvalidArgument`    | the error's text                            | `INVALID_ARGUMENT`      |
| `ErrFailedPrecondition` | `FailedPrecondition` | `failed precondition`                       | `FAILED_PRECONDITION`   |
| `ErrUnavailable`        | `Unavailable`        | `temporarily unavailable, retry later`      | `TEMPORARY_UNAVAILABLE` |
| `ErrBusy`               | `Aborted`            | `the same work is in progress, retry later` | `BUSY`                  |

The messages never carry the wrapped text, except a plain `ErrInvalidArgument`, whose text describes the client's own input — wrap it only with
client-safe context. A `FieldError` path goes through `WithFieldPathMapper` (as written when unset); a handler re-roots a domain path under its
request with `Rooted` ([`go-atlas-core/errors.MapFieldPath`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors) with `Prefixed`).
`WithCanonicalAPIRules()` installs the rules with `CamelPath` as the mapper, which leaves a rooted path unchanged. The canonical constants live in
`github.com/altessa-s/go-aip/apierror` (for example, `apierror.MessageConflict` and `apierror.ReasonVersionConflict`). Service-specific rules go after
`WithCanonicalRules`. A rule registered before it overrides a canonical one.

```go
builder.UseErrStatusOptions(
    errstatus.WithCanonicalAPIRules(),
    errstatus.WithErrorRules(errstatus.ErrorRule{Target: ErrQuota, Code: codes.ResourceExhausted, Reason: "QUOTA_EXHAUSTED"}),
)

// In a handler: report the domain field "base_url" as "provider.baseUrl".
return nil, errstatus.Rooted(err, "provider")
```

Handler unit tests run calls through the same interceptor with [errstatustest](errstatustest/README.md).

### Request-field failures

A list or update handler reports an invalid request field by returning a
[`go-atlas-core/errors.FieldError`](https://github.com/altessa-s/go-atlas-core/tree/develop/errors) whose `Err` keeps the underlying failure in the
chain; the canonical field-error row turns it into the violation below, byte-identical to `FieldViolation(path, reason)` returned by the handler. An
`etag.ErrMismatch` ([`go-atlas-core/encoding/etag.CheckVersion`](https://github.com/altessa-s/go-atlas-core/tree/develop/encoding/etag)) wraps
`ErrConflict` and needs no wrapping: it is the canonical conflict status.

| Failure                                          | Return                                                                                  | Client sees                                      |
|--------------------------------------------------|-----------------------------------------------------------------------------------------|--------------------------------------------------|
| page token error | `FieldError` on `pageToken`, reason from `pagetoken.Classify` | malformed or expired |
| negative page size (checked by the handler)      | `NewFieldError("pageSize", "must not be negative")`                                    | `pageSize: must not be negative`                 |
| `"*"` with other update-mask paths               | ``&FieldError{Field: "updateMask", Reason: `"*" must be the only path`, Err: err}``     | `updateMask: "*" must be the only path`          |
| missing etag (`etag.ErrMissing`)                 | `&FieldError{Field: "etag", Reason: "required", Err: err}`                              | `etag: required`                                 |
| malformed etag (`etag.ErrMalformed`)             | `&FieldError{Field: "etag", Reason: "invalid value", Err: err}`                         | `etag: invalid value`                            |
| etag of another version (`etag.ErrMismatch`)     | the error as is                                                                         | `Aborted`, `changed concurrently, re-read and retry`, `VERSION_CONFLICT` |

With `WithFieldPathMapper(CamelPath)` a snake_case field such as `extract.DefaultUpdateMaskFieldName` (go-aip `fieldmask/extract`) is reported as `updateMask`.

With the gRPC server factory, pass code-supplied options to the interceptor that the configuration enables through `ServerBuilder.UseErrStatusOptions`.

## Client options

| Option                     | Default | Description                                                 |
|----------------------------|---------|-------------------------------------------------------------|
| `WithStatusConverters`     | --      | Custom status-to-error converters (priority order)          |
| `WithStatusMapping`        | --      | Simple code-to-error mapping                                |
| `WithStatusConverterFunc`  | --      | Function-based status converter                             |

## Functions

| Function              | Description                                                        |
|-----------------------|--------------------------------------------------------------------|
| `DefaultFinalizer`    | Enriches errors with ErrorInfo reason codes and RequestInfo details |
| `DefaultFinalizerWithDomain` | `DefaultFinalizer` that also sets `ErrorInfo.Domain`         |
| `GrpcStatusToReasonCode` | Maps gRPC status codes to standardized reason code strings      |
