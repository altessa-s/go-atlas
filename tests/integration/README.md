# tests/integration

Integration suite for `go-atlas`, kept in its own Go module.

```
tests/integration/
├── go.mod                 # separate module — see below
├── docker-compose.yml     # the backends the suite runs against
├── leadelectit/           # coord/leadelect: multi-node election against a live broker
├── dlockit/               # coord/dlock: contended locking against a live broker, SQL servers and MongoDB
├── limiterit/             # coord/limiters: sliding-window contract on Redis and NATS vs memory, tokenbucket and budget on top
├── probfilterit/          # cache/probfilter: Bloom and Cuckoo filters on Redis Stack — membership, deletes, shared rebuilds
├── auditit/               # audit: storage conformance, atomic and idempotent batches on SQL servers
├── brokerit/              # messaging/broker: JetStream redelivery, restarts, shutdown, idempotency middleware over Redis
├── outboxit/              # reliability/outbox: transactional delivery against MongoDB, PostgreSQL, MariaDB and MySQL
├── retryqueueit/          # reliability/retryqueue: MongoDB and SQL storages — contracts, indexes, query plans, racing replicas, purge
├── mongoit/               # store/mongo: transactions abort on panic, migration failure and rollback, keyset paging with FindPage, ListCursor projections
├── sagait/                # reliability/saga: storage contracts on SQL, Mongo and Redis + orchestrator over SQL
├── idempotencyit/         # reliability/idempotency: storage contracts on Redis, Mongo, NATS and SQL + Keeper and HTTP middleware
├── cacheit/               # cache: the Redis storage and the Cache facade (singleflight, negative caching) on Redis
└── internal/livebackend/  # fixtures shared by several suites: Redis address and client, per-test names and key prefixes
```

## Why a separate module

The integration module depends on the root through `replace github.com/altessa-s/go-atlas => ../..`. It owns the MySQL test driver and live
backend fixtures. The PostgreSQL runtime and PostgreSQL integration tests both use pgx. Run these suites separately from the root's unit tests.

## Running

```bash
docker compose -f tests/integration/docker-compose.yml up -d --wait
make test-integration
docker compose -f tests/integration/docker-compose.yml down -v
```

Ports are deliberately non-default so the stack never collides with a database already running locally. Every test **skips** rather than fails when its
server is unreachable by default, so `make test-integration` is green on a machine with no Docker — check the skip count if you expect coverage.

`make test-integration` runs packages sequentially (`-p 1`); tests within each package still run in parallel. All packages share the same backend
containers, and running them together can delay requests beyond the one-second leases used by expiry tests, producing failures unrelated to correctness.

| Backend     | Default address                       | Environment override                                                         |
| ----------- | ------------------------------------- | ---------------------------------------------------------------------------- |
| ClickHouse  | `127.0.0.1:19001`                     | `CLICKHOUSE_ADDR`, `CLICKHOUSE_DB`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD` |
| MariaDB     | `127.0.0.1:13306`                     | `MARIADB_DSN`                                                                |
| MySQL       | `127.0.0.1:13307`                     | `MYSQL_DSN`                                                                  |
| PostgreSQL  | `127.0.0.1:15432`                     | `POSTGRES_DSN`                                                               |
| PgBouncer   | `127.0.0.1:16432`                     | `PGBOUNCER_DSN`                                                              |
| MongoDB     | `127.0.0.1:27019` (replica set `rs0`) | `MONGO_PORT`, `MONGO_URI`                                                    |
| Redis Stack | `127.0.0.1:16379`                     | `REDIS_ADDR`                                                                 |
| NATS        | `nats://127.0.0.1:14222`              | `NATS_URL`                                                                   |

The AIP request contracts (filter, order, projection, listing) and their live-database suites live in
[go-aip](https://github.com/altessa-s/go-aip), whose `integration/docker` module runs them against PostgreSQL, MariaDB, ClickHouse and MongoDB.

Each test creates its own throwaway table, database or index with a unique name and drops it on cleanup — two runs against the same server
never collide.

### Volume tests

`TestVolume_*` tests sit behind the `volume` build tag. They seed `ATLAS_VOLUME_ROWS` rows (default 1,000,000) per backend, check the query plans of
every hot statement for full scans and log the throughput of the bulk operations. `make test-volume` runs them with `-p 1`; `VOLUME_ROWS`,
`VOLUME_RUN` (default `^TestVolume`) and `VOLUME_TIMEOUT` (default `120m`) override the row count, the test filter and the timeout:

```bash
make test-volume VOLUME_ROWS=20000 VOLUME_RUN='^TestVolume_Dispatch'
```

### MongoDB runs as a replica set

The compose service starts `mongod --replSet rs0` and initiates the set from its own healthcheck, because `reliability/outbox` fetches and locks a batch
inside
a transaction and transactions do not exist on a standalone mongod. The service is only reported healthy once it has a primary.

Connect with **`directConnection=true`** — the default `MONGO_URI` already does. A single-node set advertises the address it sees inside its own
container (`127.0.0.1:27017`), which is not the published one, so a driver doing topology discovery from the host follows that advertisement to a port
nothing listens on and stalls until server selection times out.

When host port 27019 is taken (another project's MongoDB), move the stack's port with `MONGO_PORT`: the compose file publishes MongoDB on it and the
suites connect to it. Set it for both steps, e.g. `MONGO_PORT=27029 make integration-up` and `MONGO_PORT=27029 make test-integration`. `MONGO_URI` still
overrides the whole address. The suites that require Mongo explicitly (`sagait`, `idempotencyit`) run their Mongo cases only when `MONGO_URI` is set.

## leadelectit — leader election against a live broker

The unit tests for `coord/leadelect` run against an embedded NATS server and assert on the pieces in isolation. Three properties only exist once several
electors compete for one key through a real broker over real time, and those are what this package covers.

| Scenario                                | Asserts                                                                                            |
| --------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `SingleLeaderAmongPeers`                | Five electors, many renewals: never two claims at once, and leadership does not drift              |
| `FenceAdvancesWithRenewals`             | The fencing token moves with each renewal, so a replayed write is distinguishable from a fresh one |
| `HandoverOnGracefulStop`                | A resign releases the key immediately — the successor does not wait for expiry                     |
| `FailoverAfterAbruptLoss`               | A holder that dies without resigning still releases the lease, via server-side key expiry          |
| `PartitionedLeaderSelfDemotes`          | A node cut off from the broker stops claiming leadership unprompted, and its token drops to `0`    |
| `CallbacksFireOnTransitions`            | The became-leader callback fires on the elected node, then on its successor                        |
| `FenceMonotonicAcrossRepeatedFailovers` | The token never stalls or moves backwards across a chain of handovers                              |
| `Bucket_TTLReconciledOnAdoption`        | A pre-existing bucket without a TTL is refused, and migrated only with `WithMigrateBucketTTL`      |
| `Bucket_KeyExpiresWithoutRenewal`       | The server ages out an unrenewed election key — the mechanism every failover above rests on        |

**Sampling shows overlap, it cannot rule it out.** `Observer` polls every 25 ms, well under the 2 s lease. A round with two claimants proves mutual
exclusion broke; a clean run is evidence it held, not proof — an overlap shorter than the interval goes unseen. Where a handover completes in tens of
milliseconds the scenario holds each term briefly so the poller has something to record, and reads the fencing tokens directly rather than trusting the
recording to have caught every term.

**Two different durations govern failover.** The election TTL (`WithTTL`) bounds how long a node keeps believing it leads once it can no longer renew.
The bucket's key TTL — fixed by the provider, not derived from the election TTL — is what releases the key when a holder dies without resigning. A
failover after an abrupt loss is therefore bounded by the latter, which is why `FailoverAfterAbruptLoss` takes about ten seconds while
`HandoverOnGracefulStop` takes milliseconds.

## dlockit — contended locking against a live broker

A lock that is never contested is indistinguishable from no lock at all. These scenarios put several holders on one key through a real broker and check
that their critical sections never coincide.

The SQL provider runs the contract suite from [`coord/dlock/providers/providertest`](../../coord/dlock/providers/providertest) on PostgreSQL,
MariaDB and MySQL — each contract over its own table, with leases aged out by writing `expires_at` directly — and the same critical-section recording
with several `dlock` instances racing for one key.

The MongoDB provider runs the same contract suite and contender recording on the compose replica set, each over its own database, with leases aged out
by writing `expires_at` directly. On top of them, `mongo_integration_test.go` pins what is specific to the provider: a release keeps the lock document
and its fencing token, so the next holder continues the sequence; a lease taken `WithoutRenewal` ends one TTL after it was taken although its holder
never released it; and `EnsureIndexes` walks the `WithExpiredCleanup` TTL index through creation (grace rounded up to whole seconds), an in-place
expiry update and removal.

| Scenario                              | Asserts                                                                                     |
| ------------------------------------- | ------------------------------------------------------------------------------------------- |
| `ContendersNeverOverlap`              | Six holders, three rounds each, holding across several renewals: peak concurrency stays one |
| `Synchronize_SerializesACounter`      | A read-modify-write from five processes loses no updates                                    |
| `HeldLockRefusesOthers`               | A second acquisition is refused, and refused immediately                                    |
| `SurvivesLongerThanTheAcquireTimeout` | The acquire timeout bounds waiting, not holding                                             |
| `AbandonedLockExpires`                | A holder that dies without releasing still frees the key, via server-side expiry            |
| `FencingTokenAdvancesAcrossHolders`   | Each successive holder sees a strictly higher token                                         |
| `ReleaseLeavesNoKey`                  | A released lock is gone at once — the next contender does not wait out the TTL              |
| `Bucket_KeyTTLFollowsTheLockTTL`      | The bucket's key TTL is the configured lock TTL, not a constant                             |

**The recording proves overlap, unlike sampling.** `Critical` is not a poller: every holder reports the instant it entered and the instant it left, so
an overlap of any duration is caught rather than merely likely to be caught. Timestamps come from one process and one clock, which is what makes
comparing them sound.

**Acquisition fails fast.** The NATS provider makes a single attempt and returns `ErrLockNotHeld` when the key is taken; it does not wait for the holder
to finish. Scenarios that need to be serialized rather than rejected loop over `Synchronize` themselves.

**The context scopes the lock, not the call.** Cancel it and the lease stops being renewed and is released, so it must not end before the work the lock
guards. The acquisition attempt is bounded separately, by the provider's `WithAcquireTimeout` — folding that bound into the same context would release
the lock the moment it elapsed.

## limiterit — rate limiting against live Redis and NATS

The unit tests run the Redis provider's Lua script on miniredis and the NATS provider on an embedded server. Here the sliding-window contract runs on
the real servers, with the in-memory provider as the reference: `TestStorage_MatchesMemory` replays one call sequence on all three and compares every
admission, `Remaining` and `Reset`.

| Scenario                            | Asserts                                                                                               |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `ExactlyLimitPerWindow`             | `limit` admissions, `Remaining` counting down, `Reset` = oldest request + period, the public sentinel |
| `WindowSlidesPerRequest`            | Each request leaves the window on its own; a rejection occupies no slot                               |
| `ResetClearsTheWindow`              | `Reset` gives the whole limit back; resetting an unknown key is not an error                          |
| `KeysAreIsolated`                   | An exhausted key limits no other                                                                      |
| `ConcurrentCallersNeverExceedLimit` | 32 goroutines on one key admit at most `limit`, and the window then holds exactly `limit`             |
| `ReplicasShareOneLimit`             | The same race from two provider instances on their own connections                                    |

On top of each storage, `TestTokenBucket_RulesOverSharedStorage` drives IP, CIDR and token rules from two replicas, and `TestBudget_*` spends one budget
from two replicas, sees it come back a period later, and reports an unreachable Redis as an error rather than an exhausted budget.

**Contention costs the NATS provider latency, not correctness.** Its compare-and-swap loop retries a revision conflict after a randomized backoff, up to
`WithMaxCASAttempts` attempts (64 by default), so under the 32-goroutine race and across two replicas every call still ends in a decision: the suite
requires zero errors, and the limit is never exceeded. A call that exhausts its attempts returns `ErrContention`.

## probfilterit — Bloom and Cuckoo filters on Redis Stack

miniredis has no RedisBloom commands, so the unit tests of the Redis storages stop at structure. Here every path that touches a filter runs against
Redis Stack: the reserve script, the journaled insert, the staging reserve, the journal drain, the commit with its journal replay, the rebuild lease and
the generation-fenced delete — and the RedisBloom error texts the storages recognize, such as the "not found" of a `NOCREATE` insert.

| Scenario                                 | Asserts                                                                                               |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `Bloom_InstancesShareOneFilter`          | Two processes see each other's inserts; the filter is reserved with the configured capacity           |
| `Bloom_BatchLargerThanOneCommand`        | A batch split across several `BF.INSERT` commands loses no value                                      |
| `Bloom_FalsePositiveRate`                | Measured over 20 000 values never added, at the expected size                                         |
| `Bloom_RebuildReplacesContents`          | Streamed and collected loads both swap atomically and leave no staging key, lease or journal          |
| `Bloom_FailedRebuildKeepsContents`       | A source failing midway publishes nothing and frees the lease                                         |
| `Bloom_InsertsDuringRebuildSurvive`      | Own, peer (a burst larger than one drain batch) and through-the-commit inserts all survive            |
| `Bloom_ConcurrentRebuildIsRefused`       | A second rebuild fails fast with `ErrRebuildInProgress`; the staging key carries a TTL until promoted |
| `Bloom_SupersededRebuildDoesNotPublish`  | A rebuild whose lease lapsed is refused by the journal drain; its successor publishes                 |
| `Bloom_RecreatedFilterIsNotRebuilt`      | A deleted live key is recreated with its parameters, and no longer reports a committed rebuild        |
| `Bloom_CloseAbortsRunningRebuild`        | `Close` interrupts a rebuild, which leaves nothing behind                                             |
| `Cuckoo_InstancesShareAddsAndDeletes`    | Inserts and deletes by one process are what another sees; stats count slots, not bytes                |
| `Cuckoo_DuplicatesNeedOneDeleteEach`     | A value inserted twice stays present until deleted twice                                              |
| `Cuckoo_MissingFilter`                   | Delete, lookup and stats of an uncreated filter are answers, not errors, and create nothing           |
| `Cuckoo_RebuildReplacesContents`         | A snapshot above the capacity fits the staging filter; deletes work across the generation bump        |
| `Cuckoo_InsertsDuringRebuild`            | Own inserts are replayed; a peer's are dropped, as documented                                         |
| `Cuckoo_SupersededRebuildDoesNotPublish` | The commit script itself refuses a rebuild whose lease lapsed                                         |
| `Factory_BuildsRedisFilters`             | The factory checks the live eviction policy and rebuilds on start                                     |

The superseded scenarios delete the rebuild lease key to stand in for its expiry, so they name it with the metadata key layout of
`cache/probfilter/internal/redisfilter`; a change to that layout has to be mirrored in `harness_test.go`.

## outboxit — transactional delivery against live MongoDB and SQL servers

The unit tests for `reliability/outbox` run against in-memory stores and assert on the state the outbox *decided* to write. Whether that state survives
a real
store is a different question: the batch fetch takes its lock inside a transaction, every deadline is evaluated against the server clock, and writes are
fenced by a lock token. None of those mechanisms exists until a real MongoDB is on the other side.

The `Dispatch_*`, `Retry_*`, `Concurrency_*`, `Lifecycle_*` and `Store_*` scenarios run on every outbox store — MongoDB and the SQL store on PostgreSQL,
MariaDB and MySQL — each in its own subtest over its own tables (`forEachBackend`). The MySQL DSN deliberately sets `loc=Asia/Tokyo` with
`parseTime=true`: the SQL store must not let the driver shift an instant. The `Watch_*` and `Dedup_*` scenarios are MongoDB-only (change streams; NATS
de-duplication through the broker adapter), except `Watch_SQLNotify*`, which run on PostgreSQL through `outboxit.PGListener`, a pgx adapter of the SQL
store's `Listener`. `StorageContract` runs the [`storagetest`](../../reliability/outbox/storages/storagetest) suite on all four stores, and
`StorageContract_Watch` its `Watch` contract on MongoDB and on PostgreSQL with `WithNotify` and the pgx listener.

| Scenario                                                  | Asserts                                                                                          |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `Dispatch_DeliversSavedEvent`                             | The baseline: saved, delivered, marked sent, lease released                                      |
| `Dispatch_CommittedTransactionPublishesExactlyOnce`       | Business write and event commit together, and the event is published once                        |
| `Dispatch_AbortedTransactionPublishesNothing`             | A rolled-back write leaves no event — the dual-write gap the pattern exists to close             |
| `Dispatch_RejectedSaveLeavesTheTransactionCommittable`    | Validation runs before any store write, so a rejected batch does not force an abort              |
| `Dispatch_CompactionPublishesOnlyTheLatestPerKey`         | Superseded events are recorded as skipped, not silently dropped                                  |
| `Dispatch_SelectsOldestEventsFirstAcrossBatches`          | Each cycle selects the oldest waiting events, the ordering compaction depends on                 |
| `Retry_FailedAttemptIsPersisted`                          | A failed attempt reaches the store: attempt counted, error readable, backoff stamped             |
| `Retry_BackoffWithholdsTheEventUntilItElapses`            | The retry deadline is enforced by the server and survives a restart                              |
| `Retry_DeadLettersWhenTheBudgetRunsOut`                   | One attempt per cycle, then terminal — and never picked up again                                 |
| `Retry_PermanentFailureIsRejectedWithoutRetrying`         | A permanent error costs one attempt, not the whole budget                                        |
| `Retry_SucceedsOnceTheDestinationRecovers`                | The retry reuses the event ID, so a broker can deduplicate it                                    |
| `Concurrency_EachEventIsDeliveredOnce`                    | Four dispatchers over 60 events: no event handed to two handlers                                 |
| `Concurrency_ReclaimedLeaseCannotOverwriteTheNewerResult` | A revoked lease cannot resurrect an event another worker already delivered                       |
| `Concurrency_SweeperLeavesLiveLeasesAlone`                | An in-flight delivery is not reclaimed, so duplicates stay exceptional                           |
| `Concurrency_OverlappingCyclesOnOneInstanceCollapse`      | A cycle firing while another is in flight is a no-op                                             |
| `Lifecycle_ExpiredEventsAreNeverDispatched`               | A deadline that has passed suppresses delivery and is recorded                                   |
| `Lifecycle_CleanupKeepsDeadLetteredEvents`                | Retention sweeps successes and keeps what an operator still has to look at                       |
| `Lifecycle_StatsReportBacklogDeadLettersAndLag`           | The gauges alerting depends on, computed against the server clock                                |
| `Store_LargeKeysAndErrors`                                | Keys and handler errors over 64 KB persist; a huge error does not roll back a sibling result     |
| `Store_ErrorsWithNULPersist`                              | A handler error containing NUL persists without rolling back a sibling result                    |
| `Store_SQLWatchIsUnsupported`                             | A SQL store without a listener reports `ErrWatchUnsupported`; dispatch stays on the schedule     |
| `Store_SQLCleanupSweepsABacklogLargerThanOneChunk`        | Retention deletes 2500 old successes in chunks of 1000; fresh ones and dead letters stay         |
| `Store_SQLUnsetInstantsComeFromTheDatabaseClock`          | Undated events are created and expire by the database clock; explicit instants are kept          |
| `Store_SQLIndexesExistAndAreAddedToExistingTables`        | The catalog holds every index; `EnsureSchema` restores a missing one, also when racing           |
| `Store_SQLClaimMergesPendingAndFailedOldestFirst`         | Pending and due failed events are claimed oldest first, ties in insertion order                  |
| `Store_SQLUpdateIsRetriedAfterADeadlock`                  | A real deadlock makes the store's update the victim; it is retried and lands                     |
| `Watch_SupportsChangeStreamsOnAReplicaSet`                | The capability probe reads a real `hello` reply the way it expects to                            |
| `Watch_DeliversWithoutAPollCycle`                         | A change stream alone carries a saved event to its handler — no schedule, no manual cycle        |
| `Watch_DeliversNothingUntilTheTransactionCommits`         | The stream reports an insert at commit, so a notification can never publish an aborted write     |
| `Watch_DrainsBacklogLargerThanOneBatch`                   | A full batch re-arms the drain; the remainder does not wait for the next tick                    |
| `Watch_CoexistsWithPollingDispatchers`                    | A watcher and a poller racing one collection still deliver each event once                       |
| `Watch_SQLNotifyDeliversWithoutAPollCycle`                | On PostgreSQL, `pg_notify` alone carries a saved event to its handler                            |
| `Watch_SQLNotifyDeliversNothingUntilTheTransactionCommits` | The signal is delivered at commit; a rolled-back save produces none                             |
| `StorageContract`, `StorageContract_Watch`                | The `storagetest` contracts on every store; `Watch` on change streams and LISTEN/NOTIFY          |

**The recorder proves duplicates, it does not sample for them.** `Recorder` captures every handler invocation with its event ID, so a duplicate is two
entries naming the same event rather than a count that came out high. `Duplicates()` names them and `Timeline()` prints the whole recording into the
failure message.

**Cycles are driven explicitly, never by a scheduler.** A scenario that asserts "nothing was delivered yet" cannot share a store with a background
poller that might deliver it at any moment. The `Watch_*` scenarios are the deliberate exception: their whole point is that no cycle is scheduled and
delivery happens anyway.

**A watcher must be proven live before it is used.** `Outbox.Watch` opens its change stream asynchronously, and a change stream only reports inserts
that happen after it is listening — so a test that saved once and waited would hang whenever the save won that race. `awaitWatcher` saves probe events
until one comes back through the handler, then clears the recording; the driver offers no "stream ready" signal to wait on instead.

**Three bugs this found.** All were invisible to the unit tests, which inspect the event the outbox built rather than the document that was stored:

| Bug                                                                                                                                                                                                               | Guard                                                     |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| A failed dispatch was never written back — the concurrent batch helper drops the result of any item whose function returned an error, so attempts never grew and no event could reach a terminal status           | `Retry_FailedAttemptIsPersisted`                          |
| A dispatcher whose lock had been reclaimed could overwrite the result of the worker that took the event over                                                                                                      | `Concurrency_ReclaimedLeaseCannotOverwriteTheNewerResult` |
| Retention compared a server-clock deadline against a **client-stamped** `published_at`, so a host whose clock ran ahead of the database kept events past their window — and one running behind deleted them early | `Lifecycle_CleanupKeepsDeadLetteredEvents`                |

The third surfaced only because the Docker VM's clock sat ~40 ms behind the host's. That is the ordinary condition on a developer machine, and a badly
synced production host is off by far more; the store now stamps `published_at` with `$$NOW`, the same clock the sweep compares against.

## retryqueueit — the retry queue's MongoDB and SQL storages

The storage contract suite from [`reliability/retryqueue/storages/storagetest`](../../reliability/retryqueue/storages/storagetest) runs against the
MongoDB storage
twice — with the default flag fields and with renamed ones plus a retention — each contract over its own database. Then what only the server shows:
`EnsureIndexes` creates exactly the due-list, per-state and (with a retention of at least one second) TTL indexes, idempotently, partial on pending and
resolved items respectively, and refuses a changed retention instead of dropping the TTL index; the queries behind `ListDue` and `ListByState` are
explained over a queue holding items in every state and must be index scans with no collection scan and no in-memory sort; and eight stores on their
own clients, as service replicas, race the first `Mark` of one key and agree on the single item inserted. MongoDB's TTL monitor runs about once a
minute, so the purge itself is not waited for — the index specification is asserted instead.

The same suite runs against the SQL storage on PostgreSQL, MariaDB and MySQL, each contract over its own table created by `EnsureSchema` (twice, for
idempotence). `PurgeExpired` deletes only resolved items past the retention against the database clock, keeps one resolved at the zero time, and drains
a backlog larger than one DELETE, and the background loop of `WithPurgeInterval` does the same unprompted and stops on `Close`; `Move` under `WithTx`
rolls back and commits with the caller's transaction and a refusal leaves it usable; eight stores on their own connection pools race the first `Mark` of
one key and agree on the single row; and the factory builds the store with the schema and the configured retention.

## mongoit — store/mongo against a live replica set

`mongo_integration_test.go` checks that a transaction whose handler panics is aborted rather than committed, that a failing migration keeps the
migrations already applied unless a full rollback is requested, and that concurrent clients each migrate their own database. The test migrations
register in the global mongo-migrate registry, as an application's do; `1_marker_test.go` and `2_gate_test.go` carry the versions in their names.

`keyset_integration_test.go` pages through `FindPage` with go-aip [`pagination/pagetoken`](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken)
tokens. It checks ties broken by `_id` values of several BSON types, a filter translated by the go-aip MongoDB backend for every page, tokens refused
under a changed filter, sort or subject, a document missing the sort value (`ErrMissingKey`) and paging under a projection.
`list_cursor_projection_test.go` pages `ListCursor` under a projection, refuses a projection that drops the cursor sort key and keeps an empty
projection off the server.

## auditit — audit storage against live SQL servers

The conformance suite from [`audit/storages/storagetest`](../../audit/storages/storagetest) — `(timestamp millisecond, ID)` order, page
boundaries, continuation under inserts, `Count`, time ranges, replays and page-token bindings — runs against the SQL storage on PostgreSQL, MariaDB and
MySQL, each contract over its own table. A batch split into several INSERTs is replayed without duplicating events, and a batch whose last INSERT the
server rejects leaves no row behind.

## Scheduler

The scheduler and its storages moved to [go-horai](https://github.com/altessa-s/go-horai), whose `integration/docker` module runs the scenarios
and storage contracts against live backends.

## sagait — saga storages against live servers

The storage contract suite from [`reliability/saga/storages/storagetest`](../../reliability/saga/storages/storagetest) — insert-if-absent creation, the
version
compare-and-swap under concurrent writers, exact ID identity and the recovery predicate at full lease precision — runs against the SQL store on
PostgreSQL, MariaDB and MySQL and against the MongoDB and Redis stores, each contract over its own throwaway table, database or key prefix. An
orchestrator built through the factory, which creates the schema, then runs a compensating saga over each SQL server. The MongoDB contracts run only
with `MONGO_URI` set (`mongodb://127.0.0.1:27019/?directConnection=true` for the compose stack), so the suite never touches another project's MongoDB on
that port.

## AIP-155 request ID store conformance

Run `go test -p 1 -count=1 -v -run TestRequestID ./mongoit ./idempotencyit` from this module. Each backend runs the go-aip
`requestidtest` scenarios with its own collection, key prefix, bucket or table. Use `MONGO_PORT`, `REDIS_ADDR` and the backend addresses above
to select the integration instances. SQL covers PostgreSQL, MariaDB and MySQL when reachable.

## idempotencyit — idempotency storages against live servers

One contract suite runs on the Redis, MongoDB and NATS JetStream storages, each test over its own key prefix, database or bucket. The MongoDB backend
runs only with `MONGO_URI` set, as in `sagait`. The Redis storage fences `Complete`, `Steal` and `Release` with Lua scripts comparing the stored bytes;
every branch of both scripts — match, mismatch, missing key — runs against real Redis, with values that contain NUL bytes and are not valid UTF-8, since
that is where miniredis and Redis can disagree.

| Scenario                                  | Asserts                                                                                                    |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `Storage_AttemptLock`                     | The first caller acquires with a token; the next sees the holder's exact bytes and no token                |
| `Storage_AcceptsTheKeysTheKeeperBuilds`   | Nested v2 keys of every layer work, each on its own entry, also for colliding `:`-joined tuples            |
| `Storage_NATSRefusesKeysEscaped…`         | NATS stores a key escaped to exactly 3072 bytes; one byte more fails with `ErrKeyTooLong`                  |
| `Storage_Complete`                        | Holder token writes; a foreign or stale token and a vanished key are refused without writing               |
| `Storage_Steal`                           | A matching expectation takes over and fences the old holder; a stale one or a missing key fails            |
| `Storage_Release`                         | Only the holder's token deletes; a completed result survives its holder's release                          |
| `Storage_PerCallTTLExpiresOnTheServer`    | `AttemptLockWithTTL` ages the key out on the server; the expired holder is refused afterwards              |
| `Storage_ConfiguredTTLExpiresOnTheServer` | The storage TTL ages out a lock and a completed result, whatever the lock's own TTL was                    |
| `Storage_Concurrent*HasOneWinner`         | 32 racing `AttemptLock`, `Steal` or `Release` calls: exactly one wins                                      |
| `Storage_LargeBinaryValues`               | The lifecycle with 512 KiB random values; a difference in the last byte is detected                        |
| `Redis_*`                                 | Lua writes renew the key with the storage TTL; refused or missing-key scripts write nothing                |
| `Keeper_*`                                | Replay of a completed result, release and retry, orphan reclaim under contention, fingerprints, namespaces |
| `Middleware_*`                            | The HTTP middleware end to end: replay refused, concurrent duplicate gets 409, failure retried             |

The HTTP middleware and the gRPC interceptor compose `("transport", service, key)` and the broker wrapper `("ibk", consumer, topic, key)` in the
length-prefixed v2 format of [`reliability/idempotency.ComposeKey`](../../reliability/idempotency/README.md#storage-key-format), and the Keeper wraps
every key in
`("namespace", namespace, key)` — `idk:v2:9:namespace0:38:idk:v2:9:transport12:POST:/orders3:123`. NATS KV keys allow only `[-/_=.a-zA-Z0-9]`, so the
NATS storage escapes the final key once (`:` becomes `=3A`) and refuses one longer than 3072 bytes after escaping.

## cacheit — the cache against a live Redis

The Redis cache storage needs no Lua — each operation is one command or a `MULTI`/`EXEC` pipeline — so the suite is about expiry on the server clock,
large values and what the `Cache` facade leaves in Redis: its default TTL, `NoTTL`, the fallback's own TTL, one fallback run for many concurrent callers
of `GetWithFallback`, negative entries that answer without the fallback until they expire, and per-tenant keys.

## brokerit — messaging/broker against a live JetStream

The unit tests for `messaging/broker` drive the NATS provider through fakes. What a broker promises — redelivery, terminal outcomes, durable progress
across restarts — is decided by the server, so these scenarios run each verdict a handler can give through a real JetStream, every test on a memory
stream of its own. The idempotency scenarios keep their records in Redis under a key prefix of their own.

| Scenario                                                    | Asserts                                                                                            |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `Delivery_RoundTripCarriesDataAndHeaders`                   | Payload, subject, application headers and the creation stamp arrive; no `Nats-*` header leaks      |
| `Delivery_BatchArrivesOnceAndSettles`                       | A pipelined batch is delivered once per message and leaves nothing pending                         |
| `Delivery_NakWithBackOffWaitsTheScheduledDelay`             | Each redelivery waits the delay the schedule gives its delivery count, not the ack wait            |
| `Delivery_NakRedeliversImmediately`                         | A plain `Nak` does not wait for the ack timer                                                      |
| `Delivery_TermStopsRedeliveryAndReportsTheReason`           | A terminated message never comes back; the reason reaches the terminated advisory                  |
| `Delivery_MaxDeliverEndsRedeliveryWithAnAdvisory`           | Exactly `MaxDeliver` deliveries, then the max-deliveries advisory — the only dead-letter hook      |
| `Delivery_UnackedMessageIsRedeliveredAfterTheAckWait`       | No verdict means redelivery after the ack wait, which the message reports as `AckTimeout`          |
| `Delivery_InProgressHeartbeatsHoldOffRedelivery`            | A handler holding a message for three ack waits keeps it through `inprogress.Manager`              |
| `Delivery_PanicIsRedeliveredAndTheSubscriptionSurvives`     | A recovered panic leaves the message for redelivery and the subscription consuming                 |
| `Restart_NothingLostWhenAConsumerDiesMidStream`             | A successor on the same durable consumer receives everything an abruptly lost instance did not ack |
| `Restart_GracefulHandoverLosesAndDuplicatesNothing`         | `Unsubscribe` mid-stream drains and acks; the successor takes the rest, each message once          |
| `Shutdown_UnsubscribeWaitsForTheInFlightHandler`            | `Unsubscribe` cancels the handler context, returns only after the handler, and its ack lands       |
| `Shutdown_AbandonedInFlightMessageGoesToTheNextInstance`    | A handler that gives up on cancellation leaves its message to the next instance                    |
| `Shutdown_SubscriberLifecycleLeaksNoGoroutines`             | Repeated subscribe/unsubscribe cycles return the goroutine count to its baseline (serial test)     |
| `Idempotency_DuplicatesAreProcessedOnce`                    | Copies of one message ID run the handler once and are acknowledged without redelivery              |
| `Idempotency_PanicReleasesTheKeyForTheRedelivery`           | A panicking handler's key is released, so the first redelivery is processed                        |
| `Idempotency_ConcurrentDuplicatesAcrossTwoInstancesRunOnce` | Two instances racing for copies of one ID: one side effect per ID, the loser backs off             |
| `Idempotency_DeduplicateIDReachesTheSubscriber`             | The deduplication ID survives the wire as `deduplicate_id`, so `DefaultKeyFunc` keys on it         |
| `Idempotency_NATSKVStorageAcceptsTheMiddlewareKeys`         | The NATS KV storage locks and completes the escaped v2 key the middleware builds                   |
| `Idempotency_ConsumerScopesDoNotCollide`                    | Consumers scoped `a:b` and `a` both process messages `c` and `b:c`: no record is shared            |

The NATS provider maps `deduplicate_id` onto `Nats-Msg-Id` on publish and back onto `deduplicate_id` on receive, so the key the broker outbox sets on
every event reaches a consumer. The broker wrapper composes `("ibk", consumer, topic, key)` in the v2 key format, which NATS KV stores escaped, so a
`:` in a consumer scope or a message ID can neither break the NATS KV storage nor make two consumers share a record.

The broker outbox's own NATS path — the event ID reused as `Nats-Msg-Id`, a republish collapsed by JetStream — is covered by `outboxit` (`Dedup_*`); its
storages are MongoDB and SQL, so it has no NATS-only path to exercise here.

## PostgreSQL pools

The `postgresit` suite exercises PostgreSQL 17 directly on port 15432 and PgBouncer transaction pooling on port 16432.
Start both with `docker compose -f tests/integration/docker-compose.yml up -d --wait postgres pgbouncer`. Run
`go test -C tests/integration -race -count=3 ./postgresit/...` with both `POSTGRES_DSN` and `PGBOUNCER_DSN` set. The goose suites use isolated
databases on the direct endpoint and trace the pooler rejection before advisory lock acquisition.
When running `outboxit` with `POSTGRES_DSN` pointing to PgBouncer, set `POSTGRES_CONTROL_DSN` to a direct connection to the same database.
LISTEN consumers need a session-affine endpoint. The connection-loss test closes its own client socket.

The optional `postgres-tls` Compose service reuses the PostgreSQL image and binds only to loopback on port 15433. From the repository root,
generate a two-day CA and server certificate with local OpenSSL, then run the trusted-CA test. With the image cached this works offline.

```sh
POSTGRES_TLS_DIR=$(sh tests/integration/postgres-tls/certificates.sh)
export POSTGRES_TLS_DIR
docker compose -f tests/integration/docker-compose.yml up --pull never -d --wait postgres-tls
POSTGRES_TLS_DSN='postgres://atlas:atlas@127.0.0.1:15433/atlas' POSTGRES_TLS_CA="$POSTGRES_TLS_DIR/ca.crt" \
    go -C tests/integration test -count=1 -run '^TestFactory_TLSWithATrustedCA$' ./postgresit
docker compose -f tests/integration/docker-compose.yml rm --stop --force --volumes postgres-tls
```

Remove the temporary certificate directory after stopping the service. With an external server, set both environment variables to its DSN and CA.
The test skips only when both variables are unset.

Run the six subsystem suites with `POSTGRES_DSN` set to the endpoint under test. For example, with the compose services running:

```sh
export POSTGRES_CONTROL_DSN='postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable'
for port in 15432 16432; do
    POSTGRES_DSN="postgres://atlas:atlas@127.0.0.1:${port}/atlas?sslmode=disable" \
        go test -C tests/integration -race -count=1 -p 1 ./auditit ./dlockit ./idempotencyit ./outboxit ./sagait ./retryqueueit || exit 1
done
```
