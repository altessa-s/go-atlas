# mongo

```go
import "github.com/altessa-s/go-atlas/data/audit/storages/mongo"
```

Package `mongo` implements `audit.Storage` using MongoDB as the backing store. Supports batch writes, indexed queries, and TTL-based cleanup for
durable audit trail persistence.
