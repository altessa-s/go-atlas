# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/coord/dlock/providers/nats"
```

Maps the `nats` block of the distribution lock configuration to a [NATS JetStream lock provider](../../../../../coord/dlock/providers/nats/) for
[`factory/coord/dlock`](../..), which builds only the `sqldb` provider.

## Usage

```go
dl, err := dlockfactory.New(cfg.DistributionLock).
    UseProvider(lockconfig.DistributionLockProviderNATS, natsfactory.Builder(natsConn)).
    Build(ctx)
```

| Symbol              | Description                                                                                        |
|---------------------|----------------------------------------------------------------------------------------------------|
| `Builder(conn)`     | Provider function for the `nats` type; `bucket`, `storage` and `migrateBucketTTL` shape the bucket |
| `ErrClientRequired` | Returned when the function has no connection                                                       |
| `ErrConfigRequired` | Returned when the configuration has no `nats` block                                                |
