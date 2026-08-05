# audit

```go
import "github.com/altessa-s/go-atlas/data/audit"
```

Package `audit` provides high-performance, asynchronous user action auditing
with at-least-once delivery semantics.

Auditor is a thin facade over a `Dispatcher` (typically `dispatch.Engine`).
The caller creates and starts the dispatch engine, then passes it to `New`.
Events flow through the engine's pipeline: channel buffer, worker pool, batch
buffer, and storage write with exponential-backoff retry. Zero impact on
request latency.

## Key types

| Type / Interface | Description                                                    |
|------------------|----------------------------------------------------------------|
| `Auditor`        | Main entry point for emitting audit events (non-blocking)      |
| `Dispatcher`     | Async item dispatch interface (`Submit`, `Dropped`)            |
| `Storage`        | Persistence interface: Store, StoreBatch, Query, Count, Close  |
| `StorageSink`    | Adapts `Storage` to `dispatch.Sink` for engine construction    |
| `JSONCodec`      | Default `dispatch.Codec` for WAL durability (JSON)             |
| `Event`          | Single audit event with full context                           |
| `EventType`      | Category: api.request, business.event, data.change, auth       |
| `Action`         | Operation: create, read, update, delete, execute, login, etc.  |
| `Actor`          | Entity performing an action (user, service, api_key, etc.)     |
| `Resource`       | Target of the action with optional change tracking             |
| `Result`         | Outcome with status, code, message, and error details          |
| `Query`          | Filter criteria for querying stored events                     |

## Options

| Option                  | Default   | Description                                      |
|-------------------------|-----------|--------------------------------------------------|
| `WithServiceInfo`       | --        | Service identification (name, version)            |
| `WithLogger`            | discard   | Structured logger (`*slog.Logger`)                |
| `WithCollector`         | noop      | Prometheus metrics collector                      |
| `WithMetricsSubsystem`  | `"audit"` | Override the metrics subsystem name               |
| `WithShutdownHooks`     | process   | Scope `Start` registers its shutdown into         |

Dispatch-level options (buffer, batch, workers, retries, WAL) are configured
on the `dispatch.Engine` — see [service/dispatch](../../service/dispatch).

### Shutdown scope

By default `Start` registers the auditor's shutdown in the process-wide registry (`core/runtime.OnShutdown`), which runs once for the whole program —
an auditor registered there cannot be stopped on its own. Pass a
[`runtime.HookGroup`](../../core/runtime) when the auditor's lifetime is shorter than the process's, e.g. when a factory owns it and the caller may
tear the subsystem down and rebuild it:

```go
var hooks runtime.HookGroup

a, _ := audit.New(engine, audit.WithShutdownHooks(&hooks))
_ = a.Start()
...
_ = hooks.Shutdown(ctx) // stops this auditor, leaves the process running
```

## Usage

```go
storage := memory.New()
eng, _ := dispatch.NewEngine[*audit.Event](
    audit.StorageSink{Storage: storage},
    dispatch.WithBufferSize[*audit.Event](10000),
)
eng.Start()
defer eng.Shutdown(ctx)

auditor, _ := audit.New(eng,
    audit.WithServiceInfo(audit.ServiceInfo{Name: "my-service"}),
    audit.WithCollector(collector),
)
auditor.Start()
defer auditor.Shutdown(ctx)

// Fire-and-forget
auditor.Emit(&audit.Event{...})

// Fluent builder
auditor.NewEvent(audit.EventTypeDataChange, audit.ActionUpdate).
    WithResource(resource).
    WithSuccess().
    Emit()
```

## Subpackages

| Package                                    | Description                       |
|--------------------------------------------|-----------------------------------|
| [factory](./factory)                       | Fluent builder from config        |
| [storages/memory](./storages/memory)       | In-memory backend for dev/test    |
| [storages/mongo](./storages/mongo)         | MongoDB-backed persistent storage |
