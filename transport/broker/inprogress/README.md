# inprogress

```go
import "github.com/altessa-s/go-atlas/transport/broker/inprogress"
```

Package `inprogress` provides a `Manager` that periodically sends InProgress heartbeats for messages being processed, preventing
premature redelivery during long-running tasks. Register `RunTickCycle` with a scheduler for periodic execution.

## Key types

| Type          | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `Manager`     | Manages periodic InProgress heartbeat sending for registered `Heartbeater` values              |
| `Heartbeater` | Interface requiring `InProgress() error`; satisfied by `msg.Message`                           |

## Methods

| Method         | Description                                                                                   |
|----------------|-----------------------------------------------------------------------------------------------|
| `New`          | Create a new `Manager` with the given options                                                 |
| `Register`     | Add a `Heartbeater` with a send interval; returns a `stop` function                           |
| `RunTickCycle` | Execute a single tick cycle, sending heartbeats for all due entries                            |

## Options

| Option             | Description                                                                    |
|--------------------|--------------------------------------------------------------------------------|
| `WithLogger`       | Set the `*slog.Logger` for warning on failed InProgress calls                  |
| `WithScheduler`    | Set the task scheduler for automatic `RunTickCycle` registration               |
| `WithTickSchedule` | Set the cron schedule string for the heartbeat tick cycle task                  |
