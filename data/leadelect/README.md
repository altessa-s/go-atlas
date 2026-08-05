# leadelect

```go
import "github.com/altessa-s/go-atlas/data/leadelect"
```

Package `leadelect` provides distributed leader election for service coordination. Ensures only one instance is active as leader among multiple
replicas, with automatic re-election on node failures and a callback system for leadership transitions.

## Key types

| Type / Interface | Description                                             |
|------------------|---------------------------------------------------------|
| `Leader`         | Main leader election manager                            |
| `LeaderElector`  | Interface: LeaderId, IsLeader, Fence, NodeId, IsRunning |

## Fencing

`IsLeader` reports leadership but cannot, on its own, stop a frozen or partitioned former leader from acting on a stale belief. `Fence` closes that
gap: it returns a monotonically non-decreasing token for the current leadership term (the NATS provider sources it from the JetStream KV revision), or
`0` when this node is not a fresh leader. Thread the token into the conditional write of a downstream store — record the highest token accepted and
reject any lower one — so a zombie leader's late write is rejected even if it still believes `IsLeader`.

## Constructor

```go
le := leadelect.New(provider, key, nodeID, opts...)
```

`key` and `nodeID` are required positional arguments — the election key shared across all replicas, and this instance's unique identifier.

## Options

| Option               | Default | Description                        |
|----------------------|---------|------------------------------------|
| `WithTTL`            | 10s     | Lease duration before leadership expires |
| `WithHandlerTimeout` | 3s      | Timeout for callback execution     |
| `WithCollector`      | no-op   | Metrics collector                  |

## Subpackages

| Package                              | Description                          |
|--------------------------------------|--------------------------------------|
| [factory](./factory)                 | Configuration-based creation         |
| [providers/nats](./providers/nats)   | NATS JetStream provider              |
| [errs](./errs)                       | Error definitions                    |
