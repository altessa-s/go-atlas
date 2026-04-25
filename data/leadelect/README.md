# leadelect

```go
import "github.com/altessa-s/go-atlas/data/leadelect"
```

Package `leadelect` provides distributed leader election for service coordination. Ensures only one instance is active as leader among multiple
replicas, with automatic re-election on node failures and a callback system for leadership transitions.

## Key types

| Type / Interface | Description                                           |
|------------------|-------------------------------------------------------|
| `Leader`         | Main leader election manager                          |
| `LeaderElector`  | Interface: LeaderId, IsLeader, NodeId, IsRunning      |
| `Config`         | Election configuration (Key, TTL, NodeId)             |

## Options

| Option               | Default | Description                        |
|----------------------|---------|------------------------------------|
| `WithHandlerTimeout` | 3s      | Timeout for callback execution     |

## Subpackages

| Package                              | Description                          |
|--------------------------------------|--------------------------------------|
| [factory](./factory)                 | Configuration-based creation         |
| [providers/nats](./providers/nats)   | NATS JetStream provider              |
| [errs](./errs)                       | Error definitions                    |
