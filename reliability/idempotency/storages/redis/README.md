# redis

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/redis"
```

Package `redis` implements idempotency storage using Redis. Safe for concurrent use across multiple processes, so several service instances can share
idempotency state.

## Orphan takeover

Redis stamps lock acquisition and takeover via `TIME` in Lua. The timestamp and payload share one key and expiry. Takeover applies the
per-call lock TTL. Completion applies the configured result TTL.

## Upgrade

Stop old writers and recreate the idempotency keys on upgrade. Clear the configured idempotency key prefix before starting the new version.
