# factory

```go
import mongofactory "github.com/altessa-s/go-atlas/factory/coord/dlock/providers/mongo"
```

Maps the `mongo` block of the distribution lock configuration to a [MongoDB lock provider](../../../../../coord/dlock/providers/mongo/) for
[`factory/coord/dlock`](../..), which builds only the `sqldb` provider.

## Usage

```go
dl, err := dlockfactory.New(cfg.DistributionLock).
    UseProvider(lockconfig.DistributionLockProviderMongo, mongofactory.Builder(db)).
    Build(ctx)
```

`collection`, `ttl`, `renewRatio`, `withoutRenewal`, `expiredCleanup` and `expiredCleanupGrace` map onto the provider options; `ensureIndexes: true`
reconciles the cleanup TTL index during `Build`, otherwise building performs no I/O.

| Symbol              | Description                                                                            |
|---------------------|----------------------------------------------------------------------------------------|
| `Builder(db)`       | Provider function for the `mongo` type; the provider logs through the builder's logger |
| `ErrClientRequired` | Returned when the function has no database                                             |
| `ErrConfigRequired` | Returned when the configuration has no `mongo` block                                   |
