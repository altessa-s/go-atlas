# mongo

```go
import "github.com/altessa-s/go-atlas/data/outbox/store/mongo"
```

Package `mongo` implements `outbox.Store` using MongoDB as the backing store. Provides durable event persistence with indexed queries for
efficient batch fetch, status updates, and cleanup.
