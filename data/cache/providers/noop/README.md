# noop

```go
import "github.com/altessa-s/go-atlas/data/cache/providers/noop"
```

Package `noop` implements the cache provider interface as a no-op. Every `Get` returns a miss and every `Save` succeeds without storing anything.
Useful for testing and as a default when caching is not required.
