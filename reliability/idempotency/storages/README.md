# storages

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages"
```

Package `storages` defines the `Storage` interface for idempotency key persistence. Implementations live in subpackages and are injected into the
top-level `idempotency.Keeper` to supply the underlying storage backend.

## Key types

| Type / Interface | Description                                                    |
|------------------|----------------------------------------------------------------|
| `Storage`        | Interface: AttemptLock, AttemptLockWithTTL, Complete, Steal, Release, Delete |
| `LockLeaseStorage` | Reports that the backend expires lock leases |

`Steal(ctx, key, expected, replacement, maxLockAge, ttl)` checks the expected value and the acquired lock's age before replacing it.
A refused takeover returns the observed live value when available. A successful takeover returns a fresh token and uses the supplied lock TTL,
or the backend default for a nonpositive value. For a lease-based store, the Keeper applies the acquiring call's maximum lock duration when
setting the TTL and retries acquire an expired lock through `AttemptLockWithTTL`. `Steal` returns any current value without writing.

## Subpackages

| Package                      | Description                                             |
|------------------------------|---------------------------------------------------------|
| [memory](./memory)           | In-memory backend with TTL                              |
| [mongo](./mongo)             | MongoDB storage, joins transactions                     |
| [nats](./nats)               | NATS JetStream storage                                  |
| [redis](./redis)             | Distributed Redis storage                               |
| [sqldb](./sqldb)             | PostgreSQL, MySQL and MariaDB storage, joins transactions |
| [storagetest](./storagetest) | Conformance kit for the backends                        |

## Release contract

Implementations must atomically compare the ownership token and remove the matching entry in `Release(ctx, key, lockToken)`. A read followed by
unconditional `Delete` is insufficient. Memory uses its mutex, Redis uses compare-and-delete Lua, NATS uses `kv.Delete` with `jetstream.LastRevision`,
and MongoDB / `sqldb` use stored-byte-filtered deletes. Missing tokens fail with `ErrMissingLockState`; a missing or replaced entry fails with
`ErrLockStolen`.

## Stored state size

Storages keep the Keeper's serialized state as opaque bytes. With response replay on, the transports store the response in it (gRPC: the marshaled
message; HTTP: status, replay headers and body; at most 256 KiB by default, base64 in the JSON state), so a record can reach about 350 KiB. MongoDB
(16 MiB documents) and Redis take that as is; a NATS KeyValue bucket must allow values that large (the server's `max_payload`, 1 MiB by default).
