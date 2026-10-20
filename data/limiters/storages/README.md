# storages

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages"
```

Package `storages` defines the `Storage` interface for rate limiting backends. Implementations live in subpackages and are injected into limiters (token
bucket, budget) to supply the underlying rate tracking mechanism.

## Key types

| Type / Interface         | Description                                                                       |
|--------------------------|-----------------------------------------------------------------------------------|
| `Storage`                | Interface: Allow, Reset                                                           |
| `MultiStorage`           | Optional interface: `Storage` plus `AllowAll`, atomic admission against many keys |
| `Limit`                  | One key of a multi-key admission: `Key`, `Limit`, `Period`                        |
| `LimitInfo`              | Rate limit state with Remaining count and Reset timestamp                         |
| `KeyError`               | Failure of one limit of a multi-key call: `Index`, `Key`, wrapped cause           |
| `AllowAll(ctx, s, l...)` | Calls `MultiStorage.AllowAll`; `ErrMultiKeyUnsupported` when `s` lacks it         |
| `ValidateLimits`         | Argument check for `AllowAll` implementations                                     |
| `ErrLimitExceeded`       | Sentinel error returned when the request rate exceeds the limit                   |
| `ErrMultiKeyUnsupported` | `AllowAll` on a storage that cannot admit several keys atomically                 |
| `ErrDuplicateKey`        | A multi-key call names one key twice                                              |

## Multi-key admission

A per-host plus global limit taken as two `Allow` calls burns the per-host slot whenever the global key refuses. `AllowAll` admits a request against
every limit or against none:

- **Admission.** A slot is taken on every key; `infos[i]` is the state of `limits[i]` after it, as `Allow` reports it.
- **Refusal.** No slot is taken on any key. The error is a `*KeyError` for the lowest refusing index that wraps `ErrLimitExceeded`; `infos[i]` holds
  `Remaining` 0 exactly for the refusing keys and the free slots of the others, and `Reset` is the time the oldest request frees a slot (0 for an
  empty window).
- **Duplicates.** A key named twice fails with a `*KeyError` for the second occurrence that wraps `ErrDuplicateKey`: one key has one window, so two
  limits for it cannot both apply. An empty list, an empty key or a non-positive limit or period panics, as in `Allow`.

The `KeyError` message names the index only, never the key, which often carries client identity.

```go
infos, err := storages.AllowAll(ctx, store,
    storages.Limit{Key: "{t1}:host:example.com", Limit: 10, Period: time.Second},
    storages.Limit{Key: "{t1}:global", Limit: 100, Period: time.Second},
)
if keyErr, ok := errors.AsType[*storages.KeyError](err); ok && errors.Is(err, storages.ErrLimitExceeded) {
    // limits[keyErr.Index] refused; nothing was taken.
}
```

| Backend            | `MultiStorage` | Atomicity                                                                                   |
|--------------------|----------------|---------------------------------------------------------------------------------------------|
| [memory](./memory) | yes            | Locks the shards of the keys in index order                                                 |
| [redis](./redis)   | yes            | One Lua script; on Cluster and Ring clients the keys must share one `{hash tag}`            |
| [nats](./nats)     | no             | KeyValue cannot update several keys atomically; `AllowAll` returns `ErrMultiKeyUnsupported` |

The token-bucket and budget limiters and `transport/http/client/limiters.Chain` still take one key per storage call; call `AllowAll` directly where
one request must pass several limits. Conformance kit: [storagetest](./storagetest).

## Subpackages

| Package                      | Description                        |
|------------------------------|------------------------------------|
| [memory](./memory)           | In-memory backend with TTL         |
| [nats](./nats)               | NATS JetStream storage             |
| [redis](./redis)             | Distributed Redis storage          |
| [storagetest](./storagetest) | Conformance kit for `MultiStorage` |
