# providers

```go
import "github.com/altessa-s/go-atlas/data/uniq/providers"
```

Package `providers` defines the `Provider` interface for unique value storage backends. Implementations live in subpackages and are injected
into the top-level `uniq.Uniq` to supply the underlying key-value persistence.

## Key types

| Type / Interface | Description                                                      |
|------------------|------------------------------------------------------------------|
| `Provider`       | Interface: Add, AddWithValue, Exist, GetValue, Remove, Clear    |

## Subpackages

| Package            | Description                        |
|--------------------|------------------------------------|
| [nats](./nats)     | NATS KV provider                   |
| [noop](./noop)     | No-op provider for testing         |
| [redis](./redis)   | Redis-backed provider              |
