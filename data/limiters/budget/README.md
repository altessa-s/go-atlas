# budget

```go
import "github.com/altessa-s/go-atlas/data/limiters/budget"
```

Package `budget` implements a distributed budget limiter for outbound requests. It tracks the total number of requests per key within a configurable
time period, rejecting requests that exceed the budget with `ErrBudgetExhausted`. Reuses storage backends from `storages` (memory, Redis, NATS) for
consistent budget enforcement across multiple service replicas.

## Key types

| Type / Interface     | Description                                                       |
|----------------------|-------------------------------------------------------------------|
| `Limiter`            | Enforces a distributed request budget using a shared storage      |
| `ErrBudgetExhausted` | Sentinel error when the budget for the period is exhausted        |
| `ErrInvalidLimit`    | `New` got a limit below 1                                         |
| `ErrInvalidPeriod`   | `New` got a period shorter than `MinPeriod` (1s)                  |
| `ErrNilStorage`      | `New` got a nil storage                                           |

`New(limit, period, storage, opts...)` takes the mandatory budget as positional arguments.

## Options

| Option            | Default | Description                          |
|-------------------|---------|--------------------------------------|
| `WithCollector`   | noop    | Prometheus metrics collector         |

## Subpackages

| Package                  | Description                     |
|--------------------------|---------------------------------|
| [factory](./factory)     | Configuration-based creation    |
