# cursor_storages

Cursor storage implementations for MongoDB keyset pagination. Each subpackage provides a backend for persisting opaque cursor tokens that
encode the position in a paginated result set.

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [kvstore](./kvstore)   | KV store-based cursor storage     |
| [memory](./memory)     | In-memory cursor storage           |
| [nats](./nats)         | NATS-backed cursor storage         |
| [redis](./redis)       | Redis-backed cursor storage        |
