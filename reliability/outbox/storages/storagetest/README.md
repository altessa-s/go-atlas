# storagetest

```go
import "github.com/altessa-s/go-atlas/reliability/outbox/storages/storagetest"
```

Contract suite for `outbox.Storage` implementations. Every bundled backend runs it, and a custom backend should too: delivery rests on oldest-first
fetches that hand each event to one dispatcher, the lock-token fence of `UpdateEvents`, and the backend-clock predicates of the retry, unlock, expiry
and retention sweeps.

## Contracts

| Function                 | Verifies                                                                                                       |
|--------------------------|----------------------------------------------------------------------------------------------------------------|
| `Run`                    | Runs every contract below but `Watch` as parallel subtests, each on a fresh store; named contracts can be skipped |
| `FetchOldestFirst`       | A fetch returns ready events by `CreatedAt`, each with a lock token; locked events are not returned again      |
| `FetchBatchSize`         | A fetch takes at most the batch size, oldest first                                                             |
| `FetchInterleavesFailed` | A failed event ready for retry is fetched in creation order among pending ones                                 |
| `PayloadsRoundTrip`      | A nil payload stays nil, an empty one empty, arbitrary bytes unchanged                                         |
| `InstantsRoundTrip`      | Caller-supplied `CreatedAt` and `ExpiresAt` come back exactly at millisecond precision; the key survives        |
| `ConcurrentFetch`        | Concurrent fetches take disjoint batches: every event is handed out exactly once                               |
| `UpdateFencing`          | After the unlock sweep reclaimed a lock, a write with the revoked token is dropped; the current token applies |
| `UnlockKeepsLiveLeases`  | The unlock sweep leaves a lock younger than the expiry alone                                                   |
| `RetryBackoff`           | A failed event is withheld until `RetryAfter` elapses; one without a backoff is ready at once                  |
| `Expire`                 | `ExpireEvents` expires events past `ExpiresAt` only, which takes them out of dispatch                          |
| `DeleteKeepsDeadLetters` | The retention sweep never removes `max-attempt-reached` or `rejected` events                                   |
| `Stats`                  | Pending (failed included), in-progress and dead-letter counts; the oldest pending age on the backend clock     |
| `Watch`                  | `outbox.Watcher`: a committed save calls `notify` within `Window`; `Watch` returns nil once its context ends   |

Fixtures use millisecond instants, the precision every bundled backend keeps. The contracts drive the store directly, without business transactions;
saving inside one is covered by [`tests/integration/outboxit`](../../../../tests/integration/outboxit).

## Usage

```go
func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) outbox.Storage { return newStore(tb) })
}

func TestWatchContract(t *testing.T) {
	t.Parallel()
	storagetest.Watch(t, newWatchingStore(t)) // e.g. MongoDB on a replica set, or sqldb with WithNotify and WithListener
}
```

`tests/integration/outboxit` runs the suite against MongoDB, PostgreSQL, MariaDB and MySQL, and `Watch` against MongoDB change streams and
PostgreSQL LISTEN/NOTIFY.
