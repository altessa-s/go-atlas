# appstats

```go
import "github.com/altessa-s/go-atlas/observability/appstats"
```

Package `appstats` provides high-performance application statistics monitoring. Collects CPU, memory, network I/O,
and goroutine metrics with sub-microsecond latency using background caching and atomic reads.

## Usage

```go
// One-off collection
stats := appstats.GetApplicationStats(ctx)
fmt.Printf("CPU: %s, Memory: %s, Goroutines: %d\n",
    stats.CPU.Service, stats.Memory.Service, stats.Runtime.Goroutines)

// Periodic logging with scheduler
statsLogger := appstats.NewStatsLogger(appstats.WithLogger(logger))
sched.Register(ctx, scheduler.TaskConfig{
    ID:       "appstats-logging",
    Schedule: appstats.DefaultStatsLogSchedule, // every 5 minutes
    Func:     statsLogger.RunLogCycle,
})
```

## Functions

| Function / Type                    | Description                                              |
|------------------------------------|----------------------------------------------------------|
| `GetApplicationStats`              | Full snapshot: CPU, memory, network, runtime stats       |
| `StartMetricsCollection`           | Start background collection goroutine                    |
| `StopMetricsCollection`            | Stop background collection                               |
| `CPULoad`                          | CPU usage for service and system                         |
| `MemoryLoad`                       | Memory usage for service, system, and total              |
| `NetworkIO`                        | Bytes received and sent                                  |
| `NumGoroutines`                    | Current goroutine count                                  |
| `Uptime`                           | Duration since process start                             |
| `NewStatsLogger`                   | Create a periodic stats logger for scheduler integration |
