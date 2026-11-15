# scheduler

```go
import "github.com/altessa-s/go-atlas/transport/grpc/handlers/scheduler"
```

Package `scheduler` implements the gRPC `SchedulerService` defined in `proto/scheduler/v1/scheduler.proto` over a
[go-horai](https://github.com/altessa-s/go-horai) `scheduler.Scheduler`. It owns what go-horai leaves to the transport: CEL filters, parsed with
the go-aip `filter/parser` and translated to typed go-horai queries, and page tokens signed with a go-aip `pagination/pagetoken` codec. Scheduler domain errors
are translated
to gRPC status codes (`NotFound`, `FailedPrecondition`, `InvalidArgument`, `Internal`) by the internal `mapError` function.

## Key types

| Type / Symbol                | Description                                                                            |
|------------------------------|----------------------------------------------------------------------------------------|
| `Handler`                    | Implements `SchedulerServiceServer` by delegating to a go-horai scheduler              |
| `New(s, tokens, parser)`     | Creates a `Handler`; `tokens` implements `TokenCodec` and `parser` implements the required `FilterParser` |
| `ErrPageTokensNotConfigured` | `List`/`ListHistory` without a codec (`FailedPrecondition`)                            |
| `ErrInvalidCursor`           | A malformed, tampered, expired or foreign page token (`InvalidArgument`)               |

## RPCs

| Method         | Description                                                                      |
|----------------|----------------------------------------------------------------------------------|
| `Get`          | Returns the full state of a task by ID                                           |
| `GetStatus`    | Returns scheduler runtime state (leader status, running count, available slots)  |
| `List`         | Paginated task summaries with optional CEL filter                                |
| `ListHistory`  | Paginated execution history for a task with optional CEL filter                  |
| `Pause`        | Pauses a task                                                                    |
| `Resume`       | Resumes a paused task                                                            |
| `Disable`      | Disables a task                                                                  |
| `Enable`       | Enables a disabled task                                                          |
| `Trigger`      | Manually triggers immediate execution of a task, bypassing its cron schedule     |
| `SkipNextRun`  | Marks the next scheduled run of a task to be skipped                             |

## Filters

Supported: `&&`, `||`, `!`, comparisons (`==`, `!=`, `<`, `<=`, `>`, `>=`) and `in` between a field and literals, `startsWith` and `contains` on a
string field, and a bare boolean field (`unmanaged`). Fields are the go-horai `query.TaskFields` and `query.HistoryFields` (camelCase, as in the
proto). Anything else is `InvalidArgument`.
