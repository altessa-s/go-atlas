# factory

```go
import "github.com/altessa-s/go-atlas/data/outbox/factory"
```

Package `factory` builds `outbox.Outbox` instances from configuration objects. Infrastructure references (MongoDB database, scheduler) are
injected at construction.
