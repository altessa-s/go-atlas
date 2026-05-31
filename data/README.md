# data

Data layer for the Atlas framework. Provides storage abstractions, caching, filtering and sorting DSLs, distributed locks, leader election, rate
limiting, idempotency, outbox pattern, probabilistic filters, and unique value storage.

## Packages

| Package                      | Description                                                         |
|------------------------------|---------------------------------------------------------------------|
| [audit](./audit)             | Asynchronous user action auditing with at-least-once delivery       |
| [cache](./cache)             | Unified caching interface with multiple backends and serialization  |
| [filter](./filter)           | CEL expression parsing and database-specific query translation      |
| [idempotency](./idempotency) | Duplicate request detection using idempotency keys                 |
| [leadelect](./leadelect)     | Distributed leader election for service coordination               |
| [limiters](./limiters)       | Rate limiting interfaces and token bucket implementation           |
| [locks](./locks)             | Distributed locking abstractions                                   |
| [mongo](./mongo)             | MongoDB client wrapper with CSFLE, transactions, and cursor paging |
| [orderby](./orderby)         | AIP-132 `order_by` DSL parser with MongoDB / Meili / RediSearch translators ([deep dive](../docs/data/orderby.md)) |
| [outbox](./outbox)           | Transactional Outbox pattern for at-least-once event delivery      |
| [probfilter](./probfilter)   | Probabilistic data structures (Bloom, Cuckoo) for existence checks |
| [uniq](./uniq)               | Unique value management with pluggable storage backends            |
