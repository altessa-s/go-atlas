# context

```go
import "github.com/altessa-s/go-atlas/core/context"
```

Package `context` provides helpers for applying and guarding timeouts on `context.Context` values.

## Functions

| Function         | Description                                                          |
|------------------|----------------------------------------------------------------------|
| `ApplyTimeout`   | Add a timeout only when the context has no existing deadline         |
| `WithDefault`    | Like `ApplyTimeout` but treats a nil context as `context.Background` |
| `WithMaxTimeout` | Cap an existing deadline to a maximum duration                       |
| `OrBackground`   | Return the context as-is, or `context.Background` when nil           |

## Usage

```go
// Apply a timeout only if one isn't already set
ctx, cancel := context.ApplyTimeout(ctx, 5*time.Second)
defer cancel()

// Cap any existing deadline to at most 30 s
ctx, cancel := context.WithMaxTimeout(ctx, 30*time.Second)
defer cancel()

// Guard a nil context at function entry
ctx = context.OrBackground(ctx)
```

All functions return a safe-to-call `context.CancelFunc`, even when no new deadline is created.
