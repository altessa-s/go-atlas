# appstats

```go
import "github.com/altessa-s/go-atlas/observability/appstats"
```

Package `appstats` provides high-performance application statistics monitoring. Collects CPU, memory, network I/O, and goroutine metrics
with sub-microsecond latency using background caching and atomic reads. Integrates with the scheduler for periodic logging.

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
