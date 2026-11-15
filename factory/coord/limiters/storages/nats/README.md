# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages/nats"
```

Maps the `nats` block of the limiter storage configuration to a [NATS JetStream limiter storage](../../../../../coord/limiters/storages/nats/) for
[`factory/coord/limiters/budget`](../../budget) and [`factory/coord/limiters/tokenbucket`](../../tokenbucket), which build only the in-memory storage.

## Usage

```go
limiter, err := budgetfactory.New(cfg.BudgetLimiter).
    UseStorage(storageconfig.BackendTypeNATS, natsfactory.Builder(js)).
    Build(ctx)
```

| Symbol              | Description                                                                                                            |
|---------------------|------------------------------------------------------------------------------------------------------------------------|
| `Builder(js)`       | Storage function for the `nats` type; the bucket is created on the `Build` context, `natsContention` tunes CAS retries |
| `ErrClientRequired` | Returned when the function has no JetStream context                                                                    |
| `ErrConfigRequired` | Returned when the storage configuration has no `nats` block                                                            |
