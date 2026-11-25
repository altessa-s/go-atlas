# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/nats"
```

Maps the `nats` block of the idempotency storage configuration to a [NATS JetStream KV idempotency
storage](../../../../../reliability/idempotency/storages/nats/) for [`factory/reliability/idempotency`](../..), which builds only the in-memory storage.

## Usage

```go
keeper, err := idempotencyfactory.New(cfg.Idempotency).
    UseStorage(idempotencyconfig.StorageTypeNATS, natsfactory.Builder(js)).
    Build(ctx)
```

| Symbol              | Description                                                                                     |
|---------------------|-------------------------------------------------------------------------------------------------|
| `Builder(js)`       | Storage function for the `nats` type; keys live in `storage.nats.bucket` and expire after `ttl` |
| `ErrClientRequired` | Returned when the function has no JetStream context                                             |
| `ErrConfigRequired` | Returned when the configuration has no `storage.nats` block                                     |
