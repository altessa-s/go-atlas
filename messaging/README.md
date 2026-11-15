# messaging

Message passing between and within processes: the broker over NATS JetStream with its outbox and consumer deduplication, and the
synchronous in-process event bus with its unit of work.

## Packages

| Package                  | Description                                                                                   |
|--------------------------|-----------------------------------------------------------------------------------------------|
| [broker](./broker)       | Message broker with Transactional Outbox publishing and a NATS JetStream provider             |
| [eventbus](./eventbus)   | Synchronous, lock-free, transaction-safe in-process event bus; `uow` unit of work ([guide](../docs/messaging/eventbus.md)) |
