# scheduler

```go
import "github.com/altessa-s/go-atlas/transport/grpc/handlers/scheduler"
```

Package `scheduler` implements the gRPC `SchedulerService` defined in `proto/scheduler/v1/scheduler.proto`. `Handler` delegates to a
`sched.Scheduler` and exposes task lifecycle operations plus scheduler status queries and task history listing. `List` and `ListHistory`
accept an optional CEL filter expression that is parsed by the scheduler and evaluated by the storage backend. Scheduler domain errors are
translated to
appropriate gRPC status codes (`NotFound`, `FailedPrecondition`, `InvalidArgument`, `Internal`) by the internal `mapError` function.

## Key types

| Type / Interface | Description                                                                            |
|------------------|----------------------------------------------------------------------------------------|
| `Handler`        | Implements `SchedulerServiceServer` by delegating to a `sched.Scheduler` instance      |

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
