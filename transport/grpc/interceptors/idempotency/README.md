# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
```

Package `idempotency` provides a gRPC server interceptor for idempotent request handling. Prevents duplicate processing using an idempotency key
from gRPC metadata (default header: `Idempotency-Key`). Uses the driven interceptor pattern. Duplicate requests receive the stored response.

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
| `WithKeyFormatValidator`          | UUID v4                    | Custom key format validation function        |
| `WithEntityIDExtractor`           | nil                        | Extract entity ID from response              |
| `WithExposeEntityID`              | false (off)                | Echo the stored entity ID to duplicate callers (see below) |
| `WithStatusCreator`               | default messages           | Custom error status creation function        |

### Entity-ID exposure is opt-in

On a duplicate request whose key already resolved to a success, the interceptor can return the stored entity ID in the
`Idempotency-Key-Entity-Id` response metadata. That is **off by default**: the caller presenting a duplicate key is not verified to be the
principal that created the original entity, so echoing it unconditionally would leak another principal's entity ID to anyone who reuses (or guesses)
the key. Enable `WithExposeEntityID()` only when idempotency keys are unguessable and scoped per principal, or when the entity ID is not sensitive.
| `WithIgnoreMethods`               | --                         | Methods to skip idempotency checking         |
| `WithIgnorePatterns`              | reflection, health         | Regex patterns for methods to skip           |

## Client helpers

Helpers for outbound calls. `DeriveKey` mints a deterministic UUID v4 from a stable operation seed and a call name (typically `info.FullMethod`), so retries of one
logical operation collapse to the same key on the server while sibling downstream calls stay distinct. `WithKey` / `WithDerivedKey` attach the key to the outgoing
gRPC metadata under `DefaultIdempotencyKeyHeader` (`Idempotency-Key`), preserving any existing metadata. The derived key is compatible with `DefaultKeyValidator`;
a server overriding `WithKeyFormatValidator` to a non-UUID format will reject it — mint the key yourself and pass it through `WithKey`.

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

`UnaryClientInterceptor` automates the helpers above. The caller tags a context once with `WithOperation(ctx, operationID)` and every outbound unary call made
with that context (directly or transitively) gets a deterministic `DeriveKey(operationID, fullMethod)` stamped into the `Idempotency-Key` header. Calls without
a seed are forwarded untouched; an explicitly attached key (`WithKey` / `WithDerivedKey`) wins over the seed-based derivation.

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
