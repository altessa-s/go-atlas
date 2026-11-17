# storages

Outbox store implementations. Each subpackage provides a persistence backend for the transactional outbox event storage.

## Subpackages

| Package          | Description                                                 |
|------------------|-------------------------------------------------------------|
| [mongo](./mongo) | MongoDB-backed event storage                                |
| [sqldb](./sqldb) | PostgreSQL / MySQL / MariaDB storage through `database/sql` |
| [storagetest](./storagetest) | Contract suite every `outbox.Storage` backend runs   |
