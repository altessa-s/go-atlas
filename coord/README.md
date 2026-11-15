# coord

Distributed coordination: locks, leader election and rate limiting. Each root package is driver-free and defines its provider or storage
contract; backends live in `providers/<backend>` and `storages/<backend>`. See the [coordination guide](../docs/coordination.md).

## Packages

| Package                  | Description                                                                                       |
|--------------------------|---------------------------------------------------------------------------------------------------|
| [dlock](./dlock)         | Distributed locks with pluggable providers (NATS, MongoDB, SQL) ([guide](../docs/coord/dlock.md)) |
| [leadelect](./leadelect) | Leader election for service coordination (NATS KV)                                                |
| [limiters](./limiters)   | Rate limiting: token bucket and budget, memory/Redis/NATS storages                                |
