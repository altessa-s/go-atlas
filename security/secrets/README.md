# secrets

```go
import "github.com/altessa-s/go-atlas/security/secrets"
```

Package `secrets` provides centralized secret management with automatic caching, real-time watch capabilities, and multiple storage backends.

## Key types

| Type / Function   | Description                                          |
|--------------------|------------------------------------------------------|
| `Manager[T]`      | Central orchestrator: caching, updates, lifecycle    |
| `Provider[T]`     | Interface for storage backends                       |
| `Value[T]`        | Secret container with metadata and secure cleanup    |
| `WatchResult[T]`  | Watch operation handle with event channel            |
| `WatchEvent[T]`   | Secret change event (Created, Updated, Deleted)      |
| `Locker`          | Distributed locking interface for write operations   |
| `Static`          | Marker interface for immutable secret sets           |

## Features

- LRU caching with configurable TTL and sharded locks
- Real-time watch API with event filtering
- Scheduler-based background cache refresh (`RunUpdateCycle`)
- Concurrent secret retrieval with worker pools
- Distributed locking for write operations
- Graceful shutdown with secure memory clearing

## Subpackages

| Package                                | Description                           |
|----------------------------------------|---------------------------------------|
| [codec](./codec)                       | Key/value encoding interfaces         |
| [factory](./factory)                   | Config-based manager creation         |
| [providers/gcp](./providers/gcp)       | Google Cloud Secret Manager backend   |
| [providers/lockbox](./providers/lockbox) | Yandex Cloud Lockbox backend        |
| [providers/memory](./providers/memory) | In-memory backend for testing         |
| [providers/vault](./providers/vault)   | HashiCorp Vault KV v2 backend         |
