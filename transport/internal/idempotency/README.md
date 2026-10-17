# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/internal/idempotency"
```

Shared kernel of the idempotency twins ([`transport/http/server/middlewares/idempotency`](../../http/server/middlewares/idempotency) and
[`transport/grpc/interceptors/idempotency`](../../grpc/interceptors/idempotency)): default key validator, sentinel error, canonical header/metadata
names, and the storage-key format.

## Key symbols

| Symbol                       | Kind     | Description                                                              |
|------------------------------|----------|--------------------------------------------------------------------------|
| `DefaultKeyHeader`           | constant | `"Idempotency-Key"` — header/metadata name carrying the idempotency key  |
| `DefaultKeyStatusHeader`     | constant | `"Idempotency-Key-Status"` — status of a duplicate request               |
| `DefaultKeyEntityIDHeader`   | constant | `"Idempotency-Key-Entity-Id"` — entity ID stored with a completed key    |
| `ErrInvalidFormat`           | sentinel | Returned by `DefaultKeyValidator` for keys that are not lowercase UUIDv4 |
| `DefaultKeyValidator(key)`   | function | Validates that a key is a lowercase UUID v4                              |
| `BuildStorageKey(service,k)` | function | Composes the `("transport", service, key)` v2 storage key                |

## Storage-key ABI

`BuildStorageKey` composes the tuple `("transport", service, key)` with [`data/idempotency.ComposeKey`](../../../data/idempotency#storage-key-format)
into the length-prefixed v2 layout, so distinct `(service, key)` pairs never share a storage key:

| Transport | Service              | Storage key                                                                       |
|-----------|----------------------|-----------------------------------------------------------------------------------|
| HTTP      | `POST:/api/v1/users` | `idk:v2:9:transport18:POST:/api/v1/users36:550e8400-e29b-41d4-a716-446655440000` |
| gRPC      | `users.UserService`  | `idk:v2:9:transport17:users.UserService36:550e8400-e29b-41d4-a716-446655440000`  |

Keys built with it persist in external backends (Redis, NATS KV, memory), so the format is an external ABI. The v2 layout is frozen; an incompatible
format needs a new version and a migration. The v2 format replaced the colliding `idk:{service}:{key}` layout with a clean cut — see the [rollout
contract](../../../data/idempotency#rolling-out-the-v2-key-format). The service part is transport-specific — `{METHOD}:/{path}` for HTTP,
`package.Service` for gRPC. Both transport packages pin the resulting keys byte-for-byte with literal-string tests.

## Usage

```go
if err := idempotency.DefaultKeyValidator(key); err != nil {
    return err // errors.Is(err, idempotency.ErrInvalidFormat)
}
storageKey := idempotency.BuildStorageKey("users.UserService", key)
```
