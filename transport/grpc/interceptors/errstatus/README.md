# errstatus

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
```

Package `errstatus` provides gRPC interceptors for consistent error-to-status and status-to-error conversion. Server-side: `ErrorConverter` mappings
with optional `Finalizer` and internal LRU cache. Client-side: `StatusConverter` for translating gRPC statuses back to application errors.

## Key types

| Type / Interface  | Description                                                                     |
|-------------------|---------------------------------------------------------------------------------|
| `ErrorConverter`  | Server-side: Matcher + Convert function for transforming errors to gRPC status; `NoCache` keeps its result out of the cache |
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
| `WithCanonicalRules`       | --                    | Converters for the canonical `core/errors` sentinels and `FieldError` (see below) |
| `WithFieldPathMapper`      | nil (as written)      | Maps a canonical `FieldError.Field` to the API path its violation names (e.g. `CamelPath`) |
| `WithFinalizer`            | nil (none applied)    | Post-processing for converted errors              |
| `WithDomain`               | --                    | `ErrorInfo.Domain`; runs after the `WithFinalizer` finalizer, which it does not replace; no effect without one |
| `WithCacheSize`            | 1000                  | Maximum entries in conversion LRU cache           |
| `WithCacheDisabled`        | false                 | Completely disable error conversion caching        |
| `WithCacheOnlySentinel`    | false                 | Restrict caching to sentinel errors only          |
| `WithSentinelErrors`       | --                    | Additional sentinel errors eligible for caching    |
| `WithLogger`               | discard               | Structured logger                                  |

## Error rules

`WithErrorMapping` publishes `err.Error()` when its message is empty. `WithErrorRules` keeps the error text private instead: a rule without a `Message`
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

```go
errstatus.ServerInterceptor(
    errstatus.WithErrorRules(errstatus.ErrorRule{Target: ErrNotFound, Code: codes.NotFound, Message: "not found"}),
    errstatus.WithFieldErrorType(func(e *FieldError) (string, string) { return errstatus.CamelPath(e.Field), e.Reason }),
)
```

## Canonical rules

`WithCanonicalRules()` maps the canonical domain sentinels of [`core/errors`](../../../../core/errors/README.md), in this order (first match
wins):

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
request with `core/errors.MapFieldPath`. The messages and reasons are exported constants (`MessageConflict`, `ReasonVersionConflict`, ...).
Service-specific rules go after `WithCanonicalRules`; a rule registered before it overrides a canonical one.

```go
builder.UseErrStatusOptions(
    errstatus.WithCanonicalRules(),
    errstatus.WithFieldPathMapper(errstatus.CamelPath),
    errstatus.WithErrorRules(errstatus.ErrorRule{Target: ErrQuota, Code: codes.ResourceExhausted, Reason: "QUOTA_EXHAUSTED"}),
)

// In a handler: report the domain field "base_url" as "provider.baseUrl".
return nil, coreerrs.MapFieldPath(err, errstatus.Prefixed("provider"))
```

### Request-field failures

A list or update handler reports an invalid request field by returning a `core/errors.FieldError` whose `Err` keeps the underlying failure in the
chain; the canonical field-error row turns it into the violation below, byte-identical to `FieldViolation(path, reason)` returned by the handler.
An `etag.ErrMismatch` (`core/encoding/etag.CheckVersion`) wraps `ErrConflict` and needs no wrapping: it is the canonical conflict status.

| Failure                                          | Return                                                                                  | Client sees                                      |
|--------------------------------------------------|-----------------------------------------------------------------------------------------|--------------------------------------------------|
| page token (`data/keyset` error)                 | `&FieldError{Field: "pageToken", Reason: keyset.Classify(err).String(), Err: err}`      | `pageToken: malformed` or `pageToken: expired`   |
| negative page size (checked by the handler)      | `NewFieldError("pageSize", "must not be negative")`                                    | `pageSize: must not be negative`                 |
| `"*"` with other update-mask paths               | ``&FieldError{Field: "updateMask", Reason: `"*" must be the only path`, Err: err}``     | `updateMask: "*" must be the only path`          |
| missing etag (`etag.ErrMissing`)                 | `&FieldError{Field: "etag", Reason: "required", Err: err}`                              | `etag: required`                                 |
| malformed etag (`etag.ErrMalformed`)             | `&FieldError{Field: "etag", Reason: "invalid value", Err: err}`                         | `etag: invalid value`                            |
| etag of another version (`etag.ErrMismatch`)     | the error as is                                                                         | `Aborted`, `changed concurrently, re-read and retry`, `VERSION_CONFLICT` |

With `WithFieldPathMapper(CamelPath)` a snake_case field such as `fieldmask.DefaultUpdateMaskFieldName` is reported as `updateMask`.

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
