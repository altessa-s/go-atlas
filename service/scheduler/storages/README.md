# storages

Scheduler storage implementations. Each backend implements the `scheduler.Storage` interface (`clickhouse` only `scheduler.HistoryStorage`) for
persisting task state, execution history, and pagination queries. Choose the backend that matches your deployment requirements.

## Subpackages

| Package                | Description                                                                              |
|------------------------|------------------------------------------------------------------------------------------|
| [memory](./memory)     | In-memory storage for testing and single-instance deployments                            |
| [mongo](./mongo)       | MongoDB storage with filter pushdown and paginated queries                               |
| [redis](./redis)       | Redis storage (RedisJSON + RediSearch) with paginated queries                            |
| [sqldb](./sqldb)       | PostgreSQL / MySQL / MariaDB storage through `database/sql`, filters evaluated in SQL    |
| [clickhouse](./clickhouse) | ClickHouse `scheduler.HistoryStorage` only, for `scheduler.WithHistoryStorage`       |
| [storagetest](./storagetest) | Contract suite for `Storage` and `HistoryStorage` backends                         |

Every backend must satisfy the contract suite in [storagetest](./storagetest): every `Storage` backend here runs all of it through `storagetest.Run`,
and the ClickHouse history backend runs `storagetest.RunHistory`.
