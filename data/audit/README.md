# audit

```go
import "github.com/altessa-s/go-atlas/data/audit"
```

Package `audit` provides high-performance, asynchronous user action auditing with at-least-once delivery semantics. Events flow through a
non-blocking pipeline: channel buffer, worker pool, batch buffer, and storage write with exponential-backoff retry. Zero impact on request latency.

## Key types

| Type / Interface | Description                                                    |
|------------------|----------------------------------------------------------------|
| `Auditor`        | Main entry point for emitting audit events (non-blocking)      |
| `Storage`        | Persistence interface: Store, StoreBatch, Query, Count, Close  |
| `Event`          | Single audit event with full context                           |
| `EventType`      | Category: api.request, business.event, data.change, auth       |
| `Action`         | Operation: create, read, update, delete, execute, login, etc.  |
| `Actor`          | Entity performing an action (user, service, api_key, etc.)     |
| `Resource`       | Target of the action with optional change tracking             |
| `Result`         | Outcome with status, code, message, and error details          |
| `Query`          | Filter criteria for querying stored events                     |

## Options

| Option                  | Default   | Description                                    |
|-------------------------|-----------|------------------------------------------------|
| `WithBufferSize`        | 10 000    | Event channel buffer capacity                  |
| `WithBatchSize`         | 100       | Events per storage write                       |
| `WithFlushInterval`     | 1s        | Maximum wait before flushing a partial batch   |
| `WithWorkers`           | 2         | Concurrent dispatch goroutines                 |
| `WithRetryAttempts`     | 3         | Maximum retries per failed batch               |
| `WithRetryBackoff`      | 100ms     | Base duration for exponential backoff          |
| `WithShutdownTimeout`   | 30s       | Maximum wait during graceful shutdown          |
| `WithServiceInfo`       | --        | Service identification (name, version)         |
| `WithLogger`            | discard   | Structured logger (`*slog.Logger`)             |
| `WithOnDrop`            | nil       | Callback when an event is dropped              |
| `WithBackPressure`      | false     | Block Emit when buffer is full                 |

## Subpackages

| Package                                    | Description                       |
|--------------------------------------------|-----------------------------------|
| [middleware/grpc](./middleware/grpc)        | gRPC unary interceptor            |
| [middleware/http](./middleware/http)        | HTTP middleware                    |
| [storages/memory](./storages/memory)       | In-memory backend for dev/test    |
| [storages/mongo](./storages/mongo)         | MongoDB-backed persistent storage |
