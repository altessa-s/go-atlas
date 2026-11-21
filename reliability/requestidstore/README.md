# requestidstore

```go
import (
    "github.com/altessa-s/go-aip/requestid"
    "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
)
```

Package `requestidstore` provides storage backends for AIP-155 request IDs handled by the go-aip Guard.

## Usage

Construct the Guard once and share it across calls. Pass the fully qualified RPC method and verified principal in the scope.

```go
guard, err := requestid.New(redis.New(client))
if err != nil {
    return err
}
```

In the RPC handler, pass the scoped request to the Guard.

```go
return requestid.Do(ctx, guard, scope, req, handle)
```

## Backends

| Package | Atomic operation |
|---------|------------------|
| [redis](storages/redis) | Lua script on one hash |
| [mongo](storages/mongo) | Conditional single-document write |
| [sqldb](storages/sqldb) | Conditional row writes in PostgreSQL or MySQL |
| [nats](storages/nats) | Revision-checked JetStream KV writes |

Use `requestid.Do` on protobuf methods with a `request_id` field. The Guard chooses the key, owner token, lease and result retention. Each backend
implements the go-aip `requestid.Store` contract directly. For requests carrying an `Idempotency-Key` header, see
[Choosing between Idempotency-Key and AIP-155](../idempotency/README.md#choosing-between-idempotency-key-and-aip-155).

The [factory](../../factory/reliability/requestidstore) selects a backend from configuration.
