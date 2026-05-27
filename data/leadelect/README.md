# leadelect

```go
import "github.com/altessa-s/go-atlas/data/leadelect"
```

Package `leadelect` provides distributed leader election for service coordination. Ensures only one instance is active as leader among multiple
replicas, with automatic re-election on node failures and a callback system for leadership transitions.

## Key types

| Type / Interface | Description                                      |
|------------------|--------------------------------------------------|
| `Leader`         | Main leader election manager                     |
| `LeaderElector`  | Interface: LeaderId, IsLeader, NodeId, IsRunning |

## Constructor

```go
le := leadelect.New(provider, key, nodeID, opts...)
```

`key` and `nodeID` are required positional arguments — the election key shared across all replicas, and this instance's unique identifier.

## Options

| Option               | Default | Description                        |
|----------------------|---------|------------------------------------|
| `WithTtl`            | 10s     | Lease duration before leadership expires |
| `WithHandlerTimeout` | 3s      | Timeout for callback execution     |
| `WithCollector`      | no-op   | Metrics collector                  |

## Subpackages

| Package                              | Description                          |
|--------------------------------------|--------------------------------------|
| [factory](./factory)                 | Configuration-based creation         |
| [providers/nats](./providers/nats)   | NATS JetStream provider              |
| [errs](./errs)                       | Error definitions                    |
