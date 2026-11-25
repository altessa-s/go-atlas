# redistest

```go
import "github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
```

The package provides in-process Redis fixtures with automatic cleanup.

## Usage

`RedisClient(t)` returns a driver client and its miniredis server. Use `ServeTopology` to supply the server’s `ROLE` and `INFO cluster` replies.
