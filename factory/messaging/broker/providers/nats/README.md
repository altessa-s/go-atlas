# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/messaging/broker/providers/nats"
```

Maps the broker configuration to a [NATS JetStream broker provider](../../../../../messaging/broker/providers/nats/) and its [stream recovery
manager](../../../../../messaging/broker/providers/nats/recovery) for [`factory/messaging/broker`](../..), which builds no provider itself.

## Usage

```go
b := brokerfactory.New(&cfg.Broker).
    UseProvider(brokerconfig.ProviderNATS, natsfactory.Builder(natsConn))
provider, err := b.CreateProvider()
```

With `nats.recovery.enabled`, build the provider together with its recovery manager and pass `result.Provider` to the broker builder. The manager is
not started: call `Recovery.Start()` after `RegisterStream`.

```go
result, err := natsfactory.NewWithRecovery(natsConn, &cfg.Broker,
    brokerfactory.ProviderEnv{Logger: logger, Scheduler: scheduler})
if err != nil {
    return err
}
defer result.Close()
brk, err := b.Build(result.Provider)
```

| Symbol                                   | Description                                                                                                    |
|------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| `Builder(conn)`                          | Provider function for the `nats` type; the provider uses `conn`, which the caller owns                         |
| `NewWithRecovery(conn, cfg, env)`        | Creates the provider with its optional recovery manager; returns `ProviderWithRecovery`                        |
| `NewRecoveryManager(provider, cfg, env)` | Creates the recovery manager; returns `nil, nil` when recovery is disabled; registers tasks on `env.Scheduler` |
| `ProviderWithRecovery`                   | Bundles a `*natsprovider.NATS` provider with its optional `*recovery.Manager`; `Close()` stops the manager     |
| `ErrConnRequired`                        | Returned when the provider is built without a connection                                                       |
| `ErrProviderRequired`                    | Returned when the recovery manager is built without a provider                                                 |
