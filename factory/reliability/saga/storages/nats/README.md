# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/reliability/saga/storages/nats"
```

Maps the `nats` block of the saga storage configuration to a [NATS JetStream KV saga store](../../../../../reliability/saga/storages/nats/) for
[`factory/reliability/saga`](../..), which builds only the memory and sqldb stores.

## Usage

```go
orch, err := sagafactory.New(&cfg, def).
    UseStorage(sagaconfig.StorageTypeNATS, natsfactory.Builder(js)).
    Build(ctx)
```

| Symbol              | Description                                                                                                 |
|---------------------|-------------------------------------------------------------------------------------------------------------|
| `Builder(js)`       | Storage function for the `nats` type; instances live in `storage.nats.bucket`, `ctx` bounds bucket creation |
| `ErrClientRequired` | Returned when the function has no JetStream context                                                         |
| `ErrConfigRequired` | Returned when the configuration has no `storage.nats` block                                                 |
