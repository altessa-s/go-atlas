# inprogress

```go
import "github.com/altessa-s/go-atlas/messaging/broker/inprogress"
```

Package `inprogress` provides a `Manager` that periodically sends InProgress heartbeats for messages being processed, preventing premature redelivery
during long-running tasks. Register `RunTickCycle` with a scheduler for periodic execution.

## Key types

| Type          | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `Manager`     | Manages periodic InProgress heartbeat sending for registered `Heartbeater` values              |
| `Heartbeater` | Interface requiring `InProgress() error`; satisfied by `*msg.Message`                           |

## Methods

| Method         | Description                                                                                   |
|----------------|-----------------------------------------------------------------------------------------------|
| `New`          | Create a new `Manager` with the given options; returns an error when the scheduler rejects the heartbeat task |
| `Register`     | Add a `Heartbeater` with a send interval; returns a `stop` function                           |
| `RunTickCycle` | Execute a single tick cycle, sending heartbeats for all due entries; returns `cycle.ErrManaged` once scheduler-managed |
| `RegisterTickSchedulerFunc` | Return the tick function for a scheduler and mark the tick cycle as scheduler-managed |

## Options

| Option             | Description                                                                    |
|--------------------|--------------------------------------------------------------------------------|
| `WithMeter`        | Set the `metrics.Meter` for the `broker_inprogress` heartbeat counters (default: no-op)     |
| `WithLogger`       | Set the `*slog.Logger` for warning on failed InProgress calls                  |
| `WithScheduler`    | Set the task scheduler for automatic `RunTickCycle` registration               |
| `WithHeartbeatTimeout` | Bound a single `InProgress` call (default: `DefaultHeartbeatTimeout`, 5s); due heartbeats of one tick are sent concurrently |
| `WithTickSchedule` | Set the cron schedule string for the heartbeat tick cycle task; with `WithScheduler`, registers it under `WithTickTaskID` |
| `WithTickTaskID`   | Set the scheduler task ID of the tick cycle (default: unique per manager, `broker-inprogress-heartbeat-<n>`); managers must not share an ID |
