# requestidstore

```go
import "github.com/altessa-s/go-atlas/config/requestidstore"
```

Package `requestidstoreconfig` defines the configuration for distributed AIP-155 request ID storage.

## Storage settings

| Type | Settings |
|------|----------|
| `redis` | Key prefix |
| `mongo` | Collection |
| `sqldb` | Dialect, table, startup DDL and purge settings |
| `nats` | Bucket, replicas and purge interval |

See the [YAML template](../templates/requestidstore.yaml) for the full schema.
