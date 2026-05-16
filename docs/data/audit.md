# Audit

```go
import "github.com/altessa-s/go-atlas/data/audit"
```

Async audit trail for user actions. Events go through a buffered channel into a worker pool, get batched, and land in storage. The caller never
blocks. If the process crashes mid-flight, an optional WAL recovers what was in the buffer.

---

## What it does

| Feature                | How it works                                                     |
|------------------------|------------------------------------------------------------------|
| Async dispatch         | Buffered channel + worker pool, `Emit` returns immediately       |
| Batch writes           | Accumulates events, flushes on size or timer                     |
| Retry                  | Exponential backoff per failed batch                             |
| WAL                    | Write-ahead log on disk, replayed on restart                     |
| Fluent builder         | `NewEvent(...).WithActor(...).WithSuccess().Emit()`              |
| gRPC / HTTP hooks      | Interceptor and middleware that emit per-request events           |
| Storage                | In-memory for tests, MongoDB for prod                            |
| Metrics                | Two counters: emitted and dropped                                |

## How it's wired

```
Emit() → Dispatcher.Submit()
             ↓
       [Channel Buffer]
             ↓
         Worker Pool
             ↓
        Batch Buffer
             ↓
        StoreBatch()
             ↓
      (fail? → retry with backoff)
```

`Auditor` fills in the event ID, timestamp, and service info, then hands off to a `Dispatcher` (usually `dispatch.Engine`). The engine does the heavy
lifting: buffer, workers, batching, retries, WAL. Because the engine is a separate component, other subsystems can reuse it.

---

## Quick start

### In-memory, no WAL

```go
package main

import (
    "context"
    "log/slog"
    "time"

    "github.com/altessa-s/go-atlas/data/audit"
    "github.com/altessa-s/go-atlas/data/audit/storages/memory"
    "github.com/altessa-s/go-atlas/service/dispatch"
)

func main() {
    ctx := context.Background()

    store := memory.New()

    eng, _ := dispatch.NewEngine[*audit.Event](
        audit.StorageSink{Storage: store},
        dispatch.WithBatchSize[*audit.Event](100),
        dispatch.WithFlushInterval[*audit.Event](time.Second),
        dispatch.WithWorkers[*audit.Event](2),
    )
    eng.Start()
    defer eng.Shutdown(ctx)

    auditor, _ := audit.New(eng,
        audit.WithServiceInfo(audit.ServiceInfo{Name: "my-service", Version: "1.0"}),
        audit.WithLogger(slog.Default()),
    )
    auditor.Start()
    defer auditor.Shutdown(ctx)

    auditor.Emit(&audit.Event{
        Type:     audit.EventTypeDataChange,
        Action:   audit.ActionCreate,
        Actor:    audit.Actor{Type: audit.ActorTypeUser, ID: "user-42"},
        Resource: audit.Resource{Type: "order", ID: "ord-1"},
        Result:   audit.Result{Status: audit.ResultStatusSuccess},
    })
}
```

### Adding WAL

Pass the WAL directory and a codec to the engine. On restart it replays anything that didn't make it to storage.

```go
eng, _ := dispatch.NewEngine[*audit.Event](
    audit.StorageSink{Storage: store},
    dispatch.WithWAL[*audit.Event]("./var/audit/wal", audit.JSONCodec{}),
    dispatch.WithBatchSize[*audit.Event](200),
    dispatch.WithWorkers[*audit.Event](4),
)
```

### From config (factory)

```go
import (
    "github.com/altessa-s/go-atlas/data/audit/factory"
    dispatchfactory "github.com/altessa-s/go-atlas/service/dispatch/factory"
)

eng, _ := dispatchfactory.New[*audit.Event](&cfg.Audit.Dispatch).
    UseSink(audit.StorageSink{Storage: store}).
    UseCodec(audit.JSONCodec{}).
    UseLogger(logger).
    Build()
eng.Start()

auditor, _ := factory.New(&cfg.Audit).
    UseLogger(logger).
    UseDispatcher(eng).
    Build()
```

---

## Event model

### Event types

| Constant                   | Value              | When to use                                |
|----------------------------|--------------------|--------------------------------------------|
| `EventTypeAPIRequest`      | `api.request`      | Inbound gRPC/HTTP call                     |
| `EventTypeBusinessEvent`   | `business.event`   | Domain action (order placed, plan changed) |
| `EventTypeDataChange`      | `data.change`      | Create/update/delete on a record           |
| `EventTypeAuth`            | `auth`             | Login, logout, permission check            |
| `EventTypeSystem`          | `system`           | Migration, scheduled job, health check     |

### Actions

| Constant         | Value      |
|------------------|------------|
| `ActionCreate`   | `create`   |
| `ActionRead`     | `read`     |
| `ActionUpdate`   | `update`   |
| `ActionDelete`   | `delete`   |
| `ActionExecute`  | `execute`  |
| `ActionLogin`    | `login`    |
| `ActionLogout`   | `logout`   |
| `ActionGrant`    | `grant`    |
| `ActionRevoke`   | `revoke`   |

### Actor types

| Constant              | Value       |
|-----------------------|-------------|
| `ActorTypeUser`       | `user`      |
| `ActorTypeService`    | `service`   |
| `ActorTypeSystem`     | `system`    |
| `ActorTypeAPIKey`     | `api_key`   |
| `ActorTypeAnonymous`  | `anonymous` |

### Result statuses

| Constant               | Value     | Meaning                            |
|------------------------|-----------|------------------------------------|
| `ResultStatusSuccess`  | `success` | Completed normally                 |
| `ResultStatusFailure`  | `failure` | Client-side problem (bad input)    |
| `ResultStatusDenied`   | `denied`  | Auth rejected                      |
| `ResultStatusError`    | `error`   | Server-side error                  |

---

## Fluent builder

```go
auditor.NewEvent(audit.EventTypeDataChange, audit.ActionUpdate).
    WithActor(audit.Actor{Type: audit.ActorTypeUser, ID: "u1"}).
    WithResource(audit.Resource{Type: "document", ID: "doc-42"}).
    WithChanges(
        map[string]any{"title": "old"},
        map[string]any{"title": "new"},
        []string{"title"},
    ).
    WithSuccess().
    Emit()
```

| Method              | What it does                                     |
|---------------------|--------------------------------------------------|
| `WithActor`         | Who did it                                       |
| `WithResource`      | What was affected                                |
| `WithResult`        | Set result directly                              |
| `WithSuccess`       | Shorthand: `Result{Status: Success}`             |
| `WithFailure`       | Failure with code and message                    |
| `WithError`         | Wraps an `error` into result                     |
| `WithChanges`       | Before/after snapshots + list of changed fields  |
| `WithEventContext`  | Request ID, trace ID, correlation ID             |
| `WithMetadata`      | Arbitrary key-value pairs                        |
| `Build`             | Get the `*Event` without emitting                |
| `Emit`              | Send to auditor, returns false if dropped        |

---

## Context propagation

Interceptors and middlewares store the auditor in context automatically. You can also do it manually:

```go
ctx = audit.NewContext(ctx, auditor)

// Later, in a handler:
if a := audit.FromContext(ctx); a != nil {
    a.Emit(&audit.Event{...})
}
```

---

## gRPC interceptor

```go
import auditgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/audit"

i := auditgrpc.ServerInterceptor(auditor,
    auditgrpc.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
    auditgrpc.WithActorExtractor(func(ctx context.Context) audit.Actor {
        return extractActorFromMD(ctx)
    }),
)
```

Emits one `api.request` event per unary call. Includes method name, gRPC status code mapped to result status, duration, and request ID if present in
context.

## HTTP middleware

```go
import audithttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"

handler := audithttp.Middleware(auditor,
    audithttp.WithIgnorePaths("/health", "/ready"),
    audithttp.WithIgnoreMethods("OPTIONS"),
    audithttp.WithActorExtractor(func(r *http.Request) audit.Actor {
        return extractActorFromRequest(r)
    }),
)(next)
```

HTTP method maps to action (`GET` = read, `POST` = create, `PUT`/`PATCH` = update, `DELETE` = delete). Status code maps to result (2xx = success, 4xx =
failure, 5xx = error).

---

## Storage

| Backend  | Package                           | Use case        |
|----------|-----------------------------------|-----------------|
| Memory   | `data/audit/storages/memory`      | Dev and tests   |
| MongoDB  | `data/audit/storages/mongo`       | Production      |

Both implement:

```go
type Storage interface {
    Store(ctx context.Context, event *Event) error
    StoreBatch(ctx context.Context, events []*Event) error
    Query(ctx context.Context, query *Query) iter.Seq2[*Event, error]
    Count(ctx context.Context, query *Query) (int64, error)
    Close(ctx context.Context) error
}
```

---

## Options

| Option                  | Default   | What it sets                              |
|-------------------------|-----------|-------------------------------------------|
| `WithServiceInfo`       | --        | Name, version, instance ID                |
| `WithLogger`            | discard   | `*slog.Logger` for warnings               |
| `WithCollector`         | noop      | Prometheus collector for facade metrics   |
| `WithMetricsSubsystem`  | `"audit"` | Prometheus subsystem prefix               |

Buffer size, batch size, workers, retries, and WAL are all on the `dispatch.Engine`. See `service/dispatch`.

## Metrics

| Metric                         | Type    | What it counts                           |
|--------------------------------|---------|------------------------------------------|
| `audit_events_emitted_total`   | Counter | Events accepted by the dispatcher        |
| `audit_events_dropped_total`   | Counter | Events rejected (buffer full)            |

The engine adds its own metrics (worker count, flush latency, WAL size, sink errors) under a separate subsystem.

---

## YAML config

Full reference: `config/templates/audit.yaml`.

```yaml
audit:
  enabled: true
  storage:
    type: mongo
    mongo:
      collectionName: audit_events
      ttl: 720h
  shutdownTimeout: 30s
  dispatch:
    bufferSize: 10000
    batchSize: 100
    workers: 2
    wal:
      enabled: true
      dir: ./var/audit/wal
```

## What's provided

- Core types: `Auditor` (the facade), `Event` (one record), `Storage` (the backend interface).
- A YAML-driven builder that wires the auditor, dispatch engine, and storage from the `audit:` config block.
- Storage backends: in-memory (tests, single node) and MongoDB (durable, indexable).
- HTTP middleware and gRPC interceptor that emit one event per request, with payload/header redaction.
- An async dispatch engine — buffer, batching, retries, optional WAL — that decouples request handling from storage I/O.
