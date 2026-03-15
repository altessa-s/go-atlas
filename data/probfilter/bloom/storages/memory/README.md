# memory

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
```

Package `memory` provides an in-memory storage backend for Bloom filters. Configurable expected item count for optimal false-positive rate.
Thread-safe via `sync.RWMutex`.

## Options

| Option                  | Default    | Description                           |
|-------------------------|------------|---------------------------------------|
| `WithExpectedItems`     | `100000`   | Expected number of items in the filter |
| `WithFalsePositiveRate` | `0.01`     | Target false-positive probability      |
