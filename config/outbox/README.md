# outboxconfig

```go
import outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
```

`Config` defines dispatch schedules, retry settings, storage, and the watch mode for the transactional outbox. Start from
`outboxconfig.Default()` for standalone configuration, or set the `outbox` block in [broker configuration](../broker).

Call `Normalize` before `Validate` to fill in the default MongoDB storage or the backend settings for a selected storage type.

| Watch mode | Storage | Behavior |
|---|---|---|
| `polling` | Any | Dispatches on the configured schedule. |
| `changeStream` | MongoDB | Also wakes the dispatcher on MongoDB change notifications. |
| `notify` | PostgreSQL via `sqldb` | Also wakes the dispatcher on PostgreSQL LISTEN/NOTIFY. |

Both push modes require a dispatch schedule because retries and unlock sweeps still rely on polling. A missing schedule returns
`ErrWatchNeedsSchedule`. The factory rejects incompatible storage and watch combinations with `ErrUnsupportedWatch`.

For SQL storage, `storage.sqldb.table` accepts a plain or schema-qualified identifier with up to 63 characters per part.
`storage.sqldb.notifyChannel` accepts an unqualified identifier up to 63 characters. An empty value derives a lowercase channel from the table name.
