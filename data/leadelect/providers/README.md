# providers

```go
import "github.com/altessa-s/go-atlas/data/leadelect/providers"
```

Package `providers` defines the `Provider` interface for leader election backends. Implementations live in subpackages and are injected into the
top-level `leadelect.Leader` to supply the underlying election mechanism.

## Key types

| Type / Interface | Description                                                         |
|------------------|---------------------------------------------------------------------|
| `Provider`       | Interface: LeaderId, IsLeader, NodeId, IsRunning, Start, Stop      |
| `Config`         | Election configuration with Key, TTL, NodeId, and notification channels |

## Subpackages

| Package            | Description                        |
|--------------------|------------------------------------|
| [nats](./nats)     | NATS JetStream provider            |
