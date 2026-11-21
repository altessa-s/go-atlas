# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/requestidstore"
```

Package `factory` builds distributed AIP-155 stores from configuration.

## Dependencies

Register [Redis](storages/redis), [MongoDB](storages/mongo) or [NATS](storages/nats) with `UseStorage`. For SQL, supply the database with
`UseSQLDB`. Services own their client connections. SQL and NATS purge loops close through the scoped or process-wide shutdown hooks.
