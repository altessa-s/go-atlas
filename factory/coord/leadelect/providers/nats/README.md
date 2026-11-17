# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/coord/leadelect/providers/nats"
```

Maps the leader elector configuration to a [NATS JetStream leader election provider](../../../../../coord/leadelect/providers/nats/) for
[`factory/coord/leadelect`](../..), which builds no provider itself.

## Usage

```go
leader, err := leadelectfactory.New(cfg.LeaderElector).
    UseProvider(lockconfig.LeaderElectorProviderNATS, natsfactory.Builder(natsConn)).
    WithNodeId(nodeID).
    Build(ctx)
```

| Symbol              | Description                                                                                       |
|---------------------|---------------------------------------------------------------------------------------------------|
| `Builder(conn)`     | Provider function for the `nats` type; `storage` and `migrateBucketTTL` shape the election bucket |
| `ErrClientRequired` | Returned when the function has no connection                                                      |
