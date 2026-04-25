# providers

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers"
```

Package `providers` defines the `Provider` and `Lock` interfaces for distributed locking backends. Implementations live in subpackages and are
injected into the top-level `dlock.DLock` to supply the underlying locking mechanism.

## Key types

| Type / Interface | Description                                                         |
|------------------|---------------------------------------------------------------------|
| `Provider`       | Interface: Lock, GetLockInfo, Close                                 |
| `Lock`           | Interface: GetLockInfo, Release                                     |
| `LockInfo`       | Lock state with Key, Owner, AcquiredAt, TTL, and FencingToken       |

## Subpackages

| Package            | Description                        |
|--------------------|------------------------------------|
| [nats](./nats)     | NATS JetStream provider            |
| [noop](./noop)     | No-op provider for testing         |
