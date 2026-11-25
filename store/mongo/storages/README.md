# storages

Cursor storage implementations of `mongo.Storage` for MongoDB `ListCursor` pagination. Each subpackage provides a backend for persisting opaque cursor
tokens that encode the position in a paginated result set.

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory cursor storage           |
| [nats](./nats)         | NATS-backed cursor storage         |
| [redis](./redis)       | Redis-backed cursor storage        |

The NATS and Redis backends share JSON serialization through [internal/kvstore](./internal/kvstore), which is not a backend.
