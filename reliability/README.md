# reliability

Delivery and execution guarantees across process and service boundaries. Each root package is driver-free and declares its storage
contract; backends live in `storages/<backend>` (and `engines/<backend>` for saga), with conformance kits in `storages/storagetest`.

## Packages

| Package                      | Description                                                                           |
|------------------------------|---------------------------------------------------------------------------------------|
| [idempotency](./idempotency) | Duplicate request detection with idempotency keys ([guide](../docs/reliability/idempotency.md)) |
| [requestidstore](./requestidstore) | Distributed AIP-155 request ID storage for go-aip |
| [outbox](./outbox)           | Transactional outbox for at-least-once event delivery (MongoDB, SQL)                  |
| [saga](./saga)               | Orchestration-based saga with compensation and crash recovery ([guide](../docs/reliability/saga.md)) |
| [retryqueue](./retryqueue)   | Durable retry queue (memory, MongoDB)                                                  |

`idempotency/principalowner` binds idempotency records to the verified tenant and subject of an `auth/principal`.
