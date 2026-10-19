# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/internal/idempotency"
```

Shared kernel of the idempotency twins ([`transport/http/server/middlewares/idempotency`](../../http/server/middlewares/idempotency) and
[`transport/grpc/interceptors/idempotency`](../../grpc/interceptors/idempotency)): key validators, sentinel error, canonical header/metadata names,
the storage-key format, per-request policies, lock-outcome classification, and the record stored with a completed key for response replay.

## Key symbols

| Symbol                       | Kind     | Description                                                              |
|------------------------------|----------|--------------------------------------------------------------------------|
| `DefaultKeyHeader`           | constant | `"Idempotency-Key"` — header/metadata name carrying the idempotency key  |
| `DefaultKeyStatusHeader`     | constant | `"Idempotency-Key-Status"` — status of a duplicate request               |
| `DefaultKeyEntityIDHeader`   | constant | `"Idempotency-Key-Entity-Id"` — entity ID stored with a completed key    |
| `ErrInvalidFormat`           | sentinel | Returned by `DefaultKeyValidator` for keys that are not lowercase UUIDv4 |
| `DefaultKeyValidator(key)`   | function | Validates that a key is a lowercase UUID v4                              |
| `OpaqueKeyValidator(n)`      | function | Accepts 1..n bytes of visible ASCII (`DefaultMaxKeyLength` when n < 1)   |
| `PatternKeyValidator(re, n)` | function | Accepts 1..n bytes that `re` matches in full                             |
| `Policy`                     | type     | Per-request mode: `PolicyDefault`, `PolicySkip`, `PolicyOptional`, `PolicyMandatory` |
| `DefaultSkipVerbs`           | variable | Method-name prefixes skipped by default: Get, List, Search, Update, Delete and their Batch forms |
| `VerbMethodPolicy(m, p, v)`  | function | `PolicySkip` when the method name starts with a verb of `v` (whole word), else `p` |
| `BuildStorageKey(service,k)` | function | Composes the `("transport", service, key)` v2 storage key                |
| `DefaultMaxKeyLength`        | constant | `128` — byte bound of opaque and pattern keys when `n < 1`               |
| `Policy.Mandatory(enforce)`  | method   | Whether the policy requires a key, `enforce` deciding for `PolicyDefault` |
| `LockOutcome`                | type     | AttemptLock result: `LockAcquired`, `LockFingerprintMismatch`, `LockOwnerMismatch`, `LockStorageError`, `LockNilState`, `LockInProgress`, `LockCompleted` |
| `ClassifyLock(l, st, err)`   | function | Classifies an AttemptLock result; decodes the stored `Record` for `LockCompleted` |
| `DefaultMaxReplaySize`       | constant | `256 KiB` — default bound of a response stored for replay                |
| `Record`                     | type     | Data stored with a completed key under replay: entity ID, binding, gRPC response or HTTP status/headers/body |
| `NewRecord(entityID, b)`     | function | Base `Record` holding the entity ID and binding; the transport attaches its response |
| `EntityIDData(entityID)`     | function | Data stored without replay: the bare entity ID, `nil` when empty         |
| `DecodeRecord(data)`         | function | Reads stored data back: a string is a pre-replay entity ID, any other value decodes into a `Record` |
| `Record.HasGRPCResponse(b)`  | method   | Whether the record holds a gRPC response to replay for binding `b`       |
| `Record.HasHTTPResponse(b)`  | method   | Whether the record holds an HTTP response to replay for binding `b`      |

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

## Response replay

With replay on, a transport stores a `Record` with the completed key; without it, the bare entity ID (`EntityIDData`), the format written before
replay existed. Its JSON field names are persisted, so they are frozen. `Record.Binding` names the operation the response belongs to — the full gRPC
method, or the HTTP method and path. gRPC storage keys are scoped by service only and HTTP keys by `METHOD:path`; a response is replayed only to a
request with the same binding, and a record without one is never replayed. A response larger than the transport's configured replay size limit
(default `DefaultMaxReplaySize`) is not stored, so a repeat of its request is refused as already used.

On a repeat, `ClassifyLock` turns the AttemptLock result into a `LockOutcome` and, for `LockCompleted`, the decoded `Record`. Each transport keeps
its own answer to every outcome: status codes, fail-open policy, headers, and the replay itself.

## Usage

```go
if err := idempotency.DefaultKeyValidator(key); err != nil {
    return err // errors.Is(err, idempotency.ErrInvalidFormat)
}
storageKey := idempotency.BuildStorageKey("users.UserService", key)
```
