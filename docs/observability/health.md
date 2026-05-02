# Health

```go
import (
    "github.com/altessa-s/go-atlas/observability/health"

    grpchealth "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
    httphealth "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
)
```

A single `health.Coordinator` owns the per-service status registry, caches results, and pushes change notifications to subscribers. The HTTP handlers
(`Healthz`, `Readyz`, `Detailed`) and the gRPC `Handler` (`grpc_health_v1.HealthServer`) are thin transport adapters on top of it — they query the same
coordinator and translate its `ServingStatus` into HTTP codes or `grpc_health_v1` enum values.

---

## What it does

| Feature                    | How it works                                                                |
|----------------------------|-----------------------------------------------------------------------------|
| Per-service registry       | `Checker` implementations registered by name                                |
| Status caching             | Per-service TTL cache, default 500 ms                                       |
| Reactive subscriptions     | Sharded watcher pool fed by `RunHealthCheckCycle`                           |
| Periodic checks            | Optional cron schedule via `WithScheduler` + `WithCheckSchedule`            |
| HTTP probes                | `Healthz` / `Readyz` (coordinator-backed) and `K8sHealtz` / `K8sReadyz` (stubs) |
| HTTP detailed view         | `Detailed` lists every registered service                                   |
| gRPC                       | `Handler` exposes `Check`, `List`, `Watch` per `grpc_health_v1`             |
| Adaptive watcher buffers   | Channel size grows under high subscriber load                               |
| Concurrent check limit     | `ListStatuses` fan-out is bounded                                           |
| Lifecycle broadcast        | `Close()` pins the pull-API to `NOT_SERVING` and broadcasts to every watcher |
| Manual override            | `SetOverallStatus` flips `CheckStatus`/`CheckHealth` ahead of dependency teardown |
| Metrics                    | Cycle duration histogram, change/check counters                             |

## How it's wired

```
                ┌──────────────────────────┐
checkers ─────► │   health.Coordinator     │ ◄─── scheduler.RunHealthCheckCycle
                │  (cache + subscriptions) │
                └────────────┬─────────────┘
                             │ CheckStatus / Subscribe
              ┌──────────────┼─────────────────────┐
              ▼              ▼                     ▼
      ┌─────────────┐ ┌─────────────┐ ┌──────────────────────────┐
      │ HTTP Healthz│ │ HTTP Readyz │ │ gRPC Handler.Check/List  │
      │   /healthz  │ │   /readyz   │ │  Watch (server-stream)   │
      └─────────────┘ └─────────────┘ └──────────────────────────┘
```

Each `Checker` is owned by the consumer (database, broker, secret manager, plugin manager — packages such as `security/secrets`, `security/vault`,
`auth/oidc`, `auth/opa`, `plugins`, `data/locks/dlock`, `transport/broker`, `transport/broker/inprogress`, `transport/broker/outbox` ship one) and
registered into a shared `Coordinator`. The coordinator never blocks on a check longer than `WithCheckTimeout`; results live in a sharded cache, so
probe handlers stay cheap under load.

In-tree subsystems that self-register when supplied a `*health.Coordinator` via their `WithHealthCoordinator` option:

| Package                          | Default service name | What is probed                                                                       |
|----------------------------------|----------------------|--------------------------------------------------------------------------------------|
| `data/locks/dlock`               | `dlock`              | Lock provider connection (NATS / noop) via the `Prober` interface                    |
| `transport/broker`               | `broker`             | NATS connection state and JetStream account info via `Prober`                        |
| `transport/broker/inprogress`    | `broker_inprogress`  | Process-local readiness of the heartbeat manager                                     |
| `transport/broker/outbox`        | `broker_outbox`      | Wrapper liveness — does not probe the wrapped store or the broker `Publisher`        |

The default service-name strings double as the metrics-subsystem strings (`broker_inprogress`, `broker_outbox`), so Prometheus labels and gRPC service
names stay symmetric. Override per-instance via the corresponding `WithHealthServiceName` option (or, in the broker factory, via
`UseHealthServiceNameBroker` / `UseHealthServiceNameInProgress` / `UseHealthServiceNameOutbox`).

---

## Quick start

### Minimal — HTTP probes backed by the coordinator

```go
package main

import (
    "context"
    "log/slog"
    "net/http"

    "github.com/altessa-s/go-atlas/observability/health"

    httphealth "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
)

func main() {
    coord := health.New(health.WithLogger(slog.Default()))
    defer coord.Close()

    coord.RegisterService("database", health.Func(func(ctx context.Context) health.ServingStatus {
        if err := db.PingContext(ctx); err != nil {
            return health.StatusNotServing
        }
        return health.StatusServing
    }))

    mux := http.NewServeMux()
    // Wrap into the HTTP server's writer.ReadWriter as your transport requires.
    mux.HandleFunc("/healthz", asHTTPHandler(httphealth.Healthz(coord)))
    mux.HandleFunc("/readyz",  asHTTPHandler(httphealth.Readyz(coord)))
    mux.HandleFunc("/healthz/details", asHTTPHandler(httphealth.Detailed(coord)))

    _ = http.ListenAndServe(":8080", mux)
}
```

### gRPC

```go
import (
    "google.golang.org/grpc"
    grpchealth "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
)

stop := make(chan struct{})
gs   := grpc.NewServer()

h := grpchealth.New(coord) // panics if coord is nil
h.Register(gs, stop)        // registers grpc_health_v1.HealthServer

// On shutdown:
close(stop)                 // ends Watch streams with codes.Canceled
gs.GracefulStop()
coord.Close()               // broadcast NOT_SERVING to remaining subscribers
```

### From config (factory)

```go
import healthfactory "github.com/altessa-s/go-atlas/observability/health/factory"

coord, err := healthfactory.New(&cfg.Health).
    UseLogger(logger).
    Build()
if err != nil {
    return err
}
defer coord.Close()
```

---

## Coordinator API

| Method                                     | What it does                                                            |
|--------------------------------------------|-------------------------------------------------------------------------|
| `New(opts...)`                             | Construct a coordinator                                                 |
| `(c) RegisterService(name, Checker)`       | Add a service                                                           |
| `(c) UnregisterService(name)`              | Remove a service and clear its cache entry                              |
| `(c) ListServices() iter.Seq[string]`      | Iterate over registered names                                           |
| `(c) CheckStatus(ctx, service)`            | Read status (cached when fresh); empty `service` = overall              |
| `(c) CheckServiceHealth(ctx, service)`     | Bypass cache, run the named checker once                                |
| `(c) CheckHealth(ctx)`                     | Bypass cache, return `SERVING` only if every checker is `SERVING`       |
| `(c) ListStatuses(ctx)`                    | Fan-out check across all services (bounded by `MaxConcurrentHealthChecks`) |
| `(c) Subscribe(ctx, service)`              | Get a `Subscription` for status change updates                          |
| `(c) NotifyStatusChange(service, status)`  | Push an explicit status change to watchers and update the cache         |
| `(c) BroadcastStatus(status)`              | Push the same status to every watcher across every service              |
| `(c) SetOverallStatus(status)`             | Pin the pull API (`CheckHealth`/`CheckStatus`) to `status`; pass `StatusUnknown` to clear |
| `(c) OverallStatusOverride()`              | Read the current pinned status (or `StatusUnknown` if no override is in effect) |
| `(c) TriggerRecheckAll()`                  | Drop all cached statuses; next call re-runs the checker                 |
| `(c) RegisterHealthCheckSchedulerFunc()`   | Hand the cycle to a scheduler; direct calls then return `ErrSchedulerManaged` |
| `(c) RunHealthCheckCycle(ctx)`             | One iteration over watched services + delivery of changes               |
| `(c) GetMetrics()`                         | Snapshot of `active_watchers`, `cached_statuses`, …                     |
| `(c) Close()`                              | Idempotent shutdown; pins overall status to `NOT_SERVING` and broadcasts the same to every watcher |

### `Checker` and `Func`

```go
type Checker interface {
    CheckHealth(ctx context.Context) health.ServingStatus
}

// Function adapter — most callers register checkers like this:
coord.RegisterService("vault", health.Func(func(ctx context.Context) health.ServingStatus {
    if err := vaultClient.Health(ctx); err != nil {
        return health.StatusNotServing
    }
    return health.StatusServing
}))
```

The context passed to `CheckHealth` carries the per-check timeout configured via `WithCheckTimeout` (default 2 s). Don't ignore it.

---

## `ServingStatus`

| Constant                | String           | gRPC enum                     | HTTP code (probe handlers) |
|-------------------------|------------------|-------------------------------|----------------------------|
| `StatusUnknown`         | `UNKNOWN`        | `UNKNOWN`                     | 503                        |
| `StatusServing`         | `SERVING`        | `SERVING`                     | 200                        |
| `StatusNotServing`      | `NOT_SERVING`    | `NOT_SERVING`                 | 503                        |
| `StatusServiceUnknown`  | `SERVICE_UNKNOWN`| `SERVICE_UNKNOWN`             | 503; gRPC `Check` returns `codes.NotFound` |
| `StatusDegraded`        | `DEGRADED`       | `UNKNOWN` (no proto mapping)  | 503                        |

`String()` produces stable values you can put on the wire; the JSON body of `Healthz`/`Readyz`/`Detailed` uses these strings.

---

## HTTP handlers

`transport/http/server/handlers/health/health.go` ships two flavors.

### Stubs — `K8sHealtz` and `K8sReadyz`

```go
func K8sReadyz(rw writer.ReadWriter) {
    _ = rw.Write(Response{Status: "ok"}) // always 200, no state
}
```

Both always return `{"status":"ok"}` with HTTP 200 regardless of any state. They exist for the simplest deployments where the only signal that matters is
"the process is up and the listener is accepting".

> **Default wiring caveat.** `transport/http/server/factory` registers
> `K8sHealtz` at `/internal/healthz` and `K8sReadyz` at `/internal/readyz`
> for `WithBuiltinHandlers`. They will **not** flip to `NOT_SERVING` on
> SIGTERM, on a checker failure, or on `coord.Close()` — they don't
> know the coordinator exists. To get readiness that reflects state,
> register `httphealth.Readyz(coord)` yourself (e.g. via
> `srv.Handle("/readyz", httphealth.Readyz(coord))`); from then on
> `coord.Close()` (or an explicit `coord.SetOverallStatus(StatusNotServing)`)
> flips the probe to 503 immediately — see [Lifecycle](#lifecycle-and-shutdown).

### Coordinator-backed — `Healthz`, `Readyz`, `Detailed`

| Handler           | Behavior                                                                                       |
|-------------------|------------------------------------------------------------------------------------------------|
| `Healthz(coord)`  | `CheckStatus(ctx, service)` (`?service=` query); 200 if `SERVING`, 503 otherwise               |
| `Readyz(coord)`   | `CheckStatus(ctx, "")`; 200 only when **overall** is `SERVING`                                 |
| `Detailed(coord)` | `ListStatuses(ctx)`; JSON map of every service; 503 if any service is not `SERVING`            |

Response body shape:

```json
// Healthz / Readyz
{"status":"SERVING"}

// Detailed
{
  "status": "NOT_SERVING",
  "services": {
    "database": {"status":"SERVING"},
    "vault":    {"status":"NOT_SERVING"}
  }
}
```

---

## gRPC handler

`transport/grpc/handlers/health.Handler` implements `grpc_health_v1.HealthServer`.

| RPC      | Type             | Behavior                                                                                  |
|----------|------------------|-------------------------------------------------------------------------------------------|
| `Check`  | unary            | `coord.CheckStatus`; returns `codes.NotFound` if the service name is non-empty and unknown |
| `List`   | unary            | `coord.ListStatuses`; map of every service to `HealthCheckResponse`                       |
| `Watch`  | server-stream    | `coord.Subscribe`, sends initial status, pushes every change; suppresses duplicates       |

### Watch stream lifecycle

```
client opens Watch
        │
        ▼
Subscribe(ctx, service)
        │
        ▼
send InitialStatus
        │
        ▼ (loop)
        ├── ctx.Done()                  → ctx error
        ├── stop channel closed         → codes.Canceled "Server shutting down"
        ├── ErrWatcherLimitExceeded     → codes.ResourceExhausted at Subscribe time
        └── new status from coordinator → forward (skip if equal to last)
```

The `stop <-chan struct{}` you pass to `Handler.Register` is the shutdown signal for in-flight Watch streams. Close it before `grpcServer.GracefulStop()` so
subscribers get a clean `codes.Canceled` instead of a torn connection. `coord.Close()` is orthogonal: it broadcasts `NOT_SERVING`, which subscribers receive
as one final update before their context fires.

---

## Subscriptions

```go
sub, err := coord.Subscribe(ctx, "database")
if err != nil {
    // ErrWatcherLimitExceeded or ErrCoordinatorShutdown
    return err
}
defer sub.Close()

last := sub.InitialStatus()
for status := range sub.Updates() {
    if status == last {
        continue
    }
    last = status
    // react to status change
}
```

- `Updates()` is a buffered channel sized by `WithWatcherChannelBuffer`
  (default 10). When `activeWatchers > AdaptiveBufferThreshold`, the buffer for new subscribers grows by `AdaptiveBufferMultiplier`, capped at
  `MaxAdaptiveBuffer`.
- `notify` uses a non-blocking send; if a watcher is full, the
  notification is dropped silently. Read promptly.
- `Close()` is idempotent and safe to call from any goroutine.

`Subscribe` is the contract `Watch` uses internally — same backpressure and limits apply to the gRPC stream.

---

## Periodic check cycle

The cycle re-runs registered checkers and pushes diffs to subscribers. Two ways to drive it:

### Via the built-in scheduler hook

```go
coord := health.New(
    health.WithScheduler(sched),                  // service/scheduler
    health.WithCheckSchedule("*/5 * * * * *"),    // every 5 s
    health.WithLogger(logger),
)
```

The coordinator registers a single task (`id="health-check"`, `RunOnStart=true`, `Unmanaged=true`, `DisableHistory=true`) and after that direct calls to
`RunHealthCheckCycle` return `ErrSchedulerManaged`. Use `RegisterHealthCheckSchedulerFunc` if you want to register the cycle with a scheduler manually.

### Manually

```go
go func() {
    t := time.NewTicker(5 * time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-t.C:
            _ = coord.RunHealthCheckCycle(ctx)
        }
    }
}()
```

Cycles are mutually exclusive: a second call while one is running returns immediately. The cycle iterates **only** services that have active subscribers —
there's no point checking a service nobody is watching, and `CheckStatus` already covers ad-hoc lookups via the cache.

---

## Lifecycle and shutdown

`Coordinator.Close()` does two things in one call: it pins the overall status to `NOT_SERVING` (so every `CheckHealth`/`CheckStatus` reader — including
`httphealth.Readyz(coord)` — flips to 503 immediately) and broadcasts the same status to every active subscriber. It is **not** wired to OS signals
automatically. A typical shutdown:

```go
ctx, cancel := signal.NotifyContext(context.Background(),
    syscall.SIGTERM, syscall.SIGINT)
defer cancel()

<-ctx.Done()

// 1. Flip readiness so external traffic drains. SetOverallStatus
//    + BroadcastStatus reach pull-API readers and active Watch
//    streams; Close() does both atomically.
coord.Close()

// 2. Stop accepting new work.
close(grpcStopCh)             // ends Watch streams
grpcServer.GracefulStop()
_ = httpServer.Shutdown(ctx)
```

If you want to flip readiness *without* shutting the coordinator down yet — for example to drain traffic and let in-flight requests settle before tearing
down dependencies — call `coord.SetOverallStatus(health.StatusNotServing)` and `coord.BroadcastStatus(health.StatusNotServing)` directly; the coordinator
stays usable, and you can clear the override later with `SetOverallStatus(health.StatusUnknown)`.

> If you rely on the default `K8sReadyz` stub, step 1 has no effect
> on `/readyz`. Switch to `httphealth.Readyz(coord)` to make the change
> observable.

---

## Metrics

Subsystem `health` (override via `WithMetricsSubsystem` is not exposed today — the prefix is fixed).

| Metric                                  | Type      | What it measures                                                  |
|-----------------------------------------|-----------|-------------------------------------------------------------------|
| `health_check_cycle_duration_seconds`   | Histogram | Wall time of a single `RunHealthCheckCycle` invocation            |
| `health_status_changes_total`           | Counter   | Number of status diffs forwarded to watchers                      |
| `health_checks_performed_total`         | Counter   | Individual `Checker.CheckHealth` calls executed inside the cycle  |

Runtime gauges (live values, not Prometheus metrics) come from `Coordinator.GetMetrics()`:

| Key                     | Meaning                                              |
|-------------------------|------------------------------------------------------|
| `active_watchers`       | Watchers currently subscribed                        |
| `total_watchers`        | Same value, computed from shards (sanity check)      |
| `list_calls_in_flight`  | Concurrent `ListStatuses` invocations                |
| `cached_statuses`       | Entries in the per-service cache                     |

---

## Options

| Option                              | Default            | What it sets                                                  |
|-------------------------------------|--------------------|---------------------------------------------------------------|
| `WithLogger`                        | discard            | `*slog.Logger` for diagnostic logs                            |
| `WithCollector`                     | noop               | Prometheus collector for the metrics above                    |
| `WithWatcherChannelBuffer`          | `10`               | Base buffer per subscriber                                    |
| `WithMaxWatchersPerService`         | `1000`             | Cap; further `Subscribe` calls return `ErrWatcherLimitExceeded` |
| `WithNumShards`                     | `32`               | Watcher map shards (lock contention)                          |
| `WithMaxConcurrentHealthChecks`     | `10`               | Fan-out cap for `ListStatuses`                                |
| `WithStatusCacheTTL`                | `500 ms`           | How long `CheckStatus` reuses a result                        |
| `WithCheckTimeout`                  | `2 s`              | Per-checker timeout via `corectx.ApplyTimeout`                |
| `WithAdaptiveBufferThreshold`       | `100`              | Watcher count above which new buffers scale up                |
| `WithAdaptiveBufferMultiplier`      | `2`                | Multiplier applied above the threshold                        |
| `WithMaxAdaptiveBuffer`             | `100`              | Hard cap on the scaled buffer                                 |
| `WithScheduler`                     | nil                | `corescheduler.TaskRegistrar` to drive the cycle              |
| `WithCheckSchedule`                 | empty              | Cron expression; required alongside `WithScheduler`           |

---

## Errors

| Sentinel                    | Returned by                                                          |
|-----------------------------|----------------------------------------------------------------------|
| `ErrCoordinatorShutdown`    | `Subscribe` after `Close`                                            |
| `ErrWatcherLimitExceeded`   | `Subscribe` when `MaxWatchersPerService` is reached                  |
| `ErrSchedulerManaged`       | `RunHealthCheckCycle` after `RegisterHealthCheckSchedulerFunc`       |
| `ErrServiceNotFound`        | Helpers in this package (callers; not the coordinator itself)        |
| `ErrCheckTimeout`           | Reserved for callers wrapping their own checker timeouts              |
| `ErrInvalidServiceName`     | Reserved for callers validating registration input                   |
| `ErrNilCheckFunc`           | Reserved for callers asserting `health.Func` is non-nil              |

Wrap helpers (`WrapCheckError`, `WrapWatchError`, `WrapShutdownError`) add a service-aware operation prefix using `core/errors`.

---

## YAML config

Full reference: `config/health.go`.

```yaml
health:
  healthCheckInterval:       5s
  watcherChannelBuffer:      10
  maxWatchersPerService:     1000
  numShards:                 32
  maxConcurrentHealthChecks: 10
  statusCacheTTL:            500ms
  checkTimeout:              2s
  adaptiveBufferThreshold:   100
  adaptiveBufferMultiplier:  2
  maxAdaptiveBuffer:         100
```

The factory in `observability/health/factory` maps every field above onto its `With*` counterpart. `healthCheckInterval`
is informational — the coordinator itself does not poll on a fixed interval; either drive it via `WithScheduler` + `WithCheckSchedule` or call
`RunHealthCheckCycle` from your own loop.

---

## What's provided

- A coordinator with subscriptions, per-service `Checker` interface, and a four-state status model (`Unknown` / `Serving` / `NotServing` / `ShuttingDown`).
- A YAML-driven builder that turns the config block above into a ready coordinator.
- HTTP probe handlers — Kubernetes `livez` / `readyz`, plus richer endpoints that surface per-service status as JSON.
- A gRPC handler that implements the standard `grpc_health_v1` service against the same coordinator.

Subsystems that ship a built-in `Checker`: OIDC authentication, OPA authorization, secrets manager, Vault client, plugins host.
