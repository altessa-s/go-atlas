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

`Config` is a deliberate exception to the repo rule against public `Config` structs: it is an SPI value carrier assembled by the
`leadelect.Leader` core and passed to `Provider.Start`, not a user-facing tuning surface configured by callers.

## Subpackages

| Package            | Description                        |
|--------------------|------------------------------------|
| [nats](./nats)     | NATS JetStream provider            |
