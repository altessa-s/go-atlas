# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
```

Package `idempotency` provides a gRPC server interceptor for idempotent request handling. Prevents duplicate processing using an idempotency key from
gRPC metadata (default header: `Idempotency-Key`). Uses the driven interceptor pattern. A completed duplicate is rejected with
`codes.FailedPrecondition` by default (responses are not replayed); the stored entity ID is echoed in metadata only with `WithExposeEntityID()`.

## Key types

| Type / Interface     | Description                                                                |
|----------------------|----------------------------------------------------------------------------|
| `ErrorScenario`      | Identifies the reason for an idempotency error (Missing, Invalid, etc.)   |
| `KeyFormatValidator` | Validates idempotency key format (default: UUID v4)                        |
| `EntityIdExtractor`  | Extracts entity ID from successful response for status metadata            |
| `StatusCreator`      | Creates custom gRPC status errors for idempotency scenarios                |

## Options

| Option                            | Default                    | Description                                 |
|-----------------------------------|----------------------------|---------------------------------------------|
| `WithIdempotencyKeyHeader`        | `Idempotency-Key`          | Metadata header name for the key            |
| `WithIdempotencyKeyStatusMetadata`| `Idempotency-Key-Status`   | Response metadata key for status            |
| `WithIdempotencyKeyEntityIDMetadata`| `Idempotency-Key-Entity-Id`| Response metadata key for entity ID        |
| `WithFallbackBehavior`            | Deny                       | Behavior on storage failure (Allow/Deny/Error) |
| `WithEnforceMandatory`            | false                      | When true, require idempotency key on all requests |
| `WithKeyFormatValidator`          | UUID v4                    | Key format check; `OpaqueKeyValidator(n)` / `PatternKeyValidator(re, n)` accept non-UUID keys |
| `WithKeyField`                    | "" (header only)           | String request field carrying the key (such as `request_id`); wins over the header |
| `WithMethodClassifier`            | nil                        | Per-method `Policy` (skip, optional, mandatory, default); `VerbMethodClassifier(p)` |
| `WithResponseReplay`              | off                        | Return the stored response to a repeat of a completed unary call (see below) |
| `WithMaxReplaySize`               | `DefaultMaxReplaySize` (256 KiB) | Largest stored response (marshaled); larger ones are not stored |
| `WithEntityIDExtractor`           | nil                        | Extract entity ID from response              |
| `WithExposeEntityID`              | false (off)                | Echo the stored entity ID to duplicate callers (see below) |
| `WithRequestFingerprint`          | true                       | Refuse a key reused for a different request (see below) |
| `WithStatusCreator`               | default messages           | Custom error status creation function        |
| `WithIgnoreMethods`               | --                         | Methods to skip idempotency checking         |
| `WithIgnorePatterns`              | reflection, health         | Regex patterns for methods to skip           |

### Entity-ID exposure is opt-in

On a duplicate request whose key already resolved to a success, the interceptor can return the stored entity ID in the `Idempotency-Key-Entity-Id`
response metadata. That is **off by default**: the caller presenting a duplicate key is not verified to be the principal that created the original
entity, so echoing it unconditionally would leak another principal's entity ID to anyone who reuses (or guesses) the key. Enable `WithExposeEntityID()`
(YAML `exposeEntityID: true`) only when idempotency keys are unguessable and scoped per principal, or when the entity ID is not sensitive.

## Key sources and method policies

The key is read from the request field named by `WithKeyField` (proto or JSON name of a singular `string` field, such as `request_id`) when it
holds a non-empty value, otherwise from the metadata header. Keys must pass the `KeyFormatValidator`: a lowercase UUID v4 by default,
`OpaqueKeyValidator(maxLen)` for client request IDs (visible ASCII, at most `maxLen` bytes, default 128) or `PatternKeyValidator(re, maxLen)` for
a regular expression matching the whole key. A `MethodClassifier`, run after the ignore lists, returns a `Policy` per method: `PolicySkip` (no
check), `PolicyOptional` (never required), `PolicyMandatory` (always required) or `PolicyDefault` (`WithEnforceMandatory` decides).
`VerbMethodClassifier(p)` applies `p` to Create, BatchCreate and custom verbs and skips the methods whose name starts with a skip verb:
`DefaultSkipVerbs` (Get, List, Search, Update, Delete and their Batch forms) unless `WithSkipVerbs` supplies others. In YAML:
`idempotencyKeyField`, `keyFormat` (`uuidv4`, `opaque`, `pattern`), `keyPattern`, `maxKeyLength`, `methodClassifier: verbs`, `mandatoryMethods`
and `mandatoryPatterns`.

## Response replay

With `WithResponseReplay()` (YAML `responseReplay: true`) a completed unary call stores its response — wrapped in `google.protobuf.Any`,
deterministically marshaled, at most `WithMaxReplaySize` bytes — with the key, and a repeat of the request receives that response with an `OK`
status (and `Idempotency-Key-Status: success`) without running the handler, instead of `FAILED_PRECONDITION`. A repeat while the
first call runs stays `ABORTED`; a failed call stores nothing, so its repeat runs again. The fingerprint and owner checks run first, so a different
request or another owner's key never sees the response. Storage keys are scoped by service, so the record is also bound to the full method,
independently of fingerprinting: a repeat of another method of the service is never answered with it, and the replayed message must be the
method's registered response type. A response larger than the cap, a record written before replay (a bare entity ID) or without a method binding,
a response type that is not (or no longer) the method's, and streaming calls keep the `FAILED_PRECONDITION` answer. Replay is off by default because the repeat's
caller receives the original response: enable it with keys scoped per principal (`idempotency.WithOwner`).

## Storage keys

The interceptor passes the Keeper the tuple `("transport", "package.Service", key)` in the length-prefixed v2 format of
[`data/idempotency.ComposeKey`](../../../../data/idempotency#storage-key-format) — every method of `users.UserService` with key `550e8400-…` shares
`idk:v2:9:transport17:users.UserService36:550e8400-…` — and the Keeper composes `("namespace", namespace, key)` around it before storage. Keys are
scoped by service, and the bytes of the key cannot reach another service's record. The format replaced `idk:{service}:{key}` with a clean cut: follow
the [rollout contract](../../../../data/idempotency#rolling-out-the-v2-key-format) when upgrading.

## Request fingerprints and owners

Each key records a fingerprint of the method and the deterministically marshaled request. A key reused for a different request is refused with
`InvalidArgument` and reason `IDEMPOTENCY_KEY_MISMATCH` instead of being treated as a replay; `WithRequestFingerprint(false)` (YAML
`disableRequestFingerprint: true`) turns this off. When the Keeper records owners (`idempotency.WithOwner`), a key of another caller is refused with
`PermissionDenied` and reason `IDEMPOTENCY_KEY_FORBIDDEN`, and nothing of its record is returned. A `ReplayAuthorizer` refusal that carries a gRPC
status (for example `NotFound`, to hide that the record exists) is answered with that status instead. Both cases map to the `ErrorIDKMismatch` and
`ErrorIDKForbidden` scenarios of a custom `StatusCreator`.

## Client helpers

Helpers for outbound calls. `DeriveKey` mints a deterministic UUID v4 from a stable operation seed and a call name (typically `info.FullMethod`), so
retries of one logical operation collapse to the same key on the server while sibling downstream calls stay distinct. `WithKey` / `WithDerivedKey`
attach the key to the outgoing gRPC metadata under `DefaultIdempotencyKeyHeader` (`Idempotency-Key`), preserving any existing metadata. The derived key
is compatible with `DefaultKeyValidator`; a server overriding `WithKeyFormatValidator` to a non-UUID format will reject it — mint the key yourself and
pass it through `WithKey`.

| Function                                  | Description                                                                            |
|-------------------------------------------|----------------------------------------------------------------------------------------|
| `DeriveKey(seed, call) string`            | Deterministic lowercase UUID v4 from `(seed, call)`; namespaced and NUL-separated.     |
| `WithKey(ctx, key) context.Context`       | Attaches `key` to outgoing gRPC metadata; preserves existing entries.                  |
| `WithDerivedKey(ctx, seed, call)`         | Shorthand for `WithKey(ctx, DeriveKey(seed, call))`.                                   |

```go
ctx = idempotency.WithDerivedKey(ctx, operationID, "/users.v1.UserService/Update")
resp, err := client.Update(ctx, req)
```

## Client interceptor

`UnaryClientInterceptor` automates the helpers above. The caller tags a context once with `WithOperation(ctx, operationID)` and every outbound unary
call made with that context (directly or transitively) gets a deterministic `DeriveKey(operationID, fullMethod)` stamped into the `Idempotency-Key`
header. Calls without a seed are forwarded untouched; an explicitly attached key (`WithKey` / `WithDerivedKey`) wins over the seed-based derivation.

| Option                              | Default                       | Description                                                            |
|-------------------------------------|-------------------------------|------------------------------------------------------------------------|
| `WithClientIdempotencyKeyHeader`    | `Idempotency-Key`             | Outgoing metadata header to stamp.                                     |
| `WithClientSeedExtractor`           | `OperationFromContext`        | Source of the per-operation seed; return `ok=false` to skip stamping.  |
| `WithClientMethodFilter`            | admit every method            | Per-method predicate; return `false` to skip stamping for that method. |

```go
conn, err := grpc.NewClient(target,
    grpc.WithTransportCredentials(creds),
    grpc.WithUnaryInterceptor(idempotency.UnaryClientInterceptor()),
)
// ...
ctx = idempotency.WithOperation(ctx, operationID)
resp, err := client.Update(ctx, req)
```
