# nats

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
```

Package `nats` implements distributed lock provider using NATS JetStream key-value store with TTL-based lease management for automatic lock expiry
on holder failure.
