# tests/integration

Integration suite for `go-atlas`, kept in its own Go module.

```
tests/integration/
├── go.mod                 # separate module — see below
├── docker-compose.yml     # the backends the suite runs against
├── filterit/              # data/filter: shared corpus + one adapter per backend
├── leadelectit/           # data/leadelect: multi-node election against a live broker
├── dlockit/               # data/locks/dlock: contended locking against a live broker and SQL servers
├── limiterit/             # data/limiters: sliding-window contract on Redis and NATS vs memory, tokenbucket and budget on top
├── probfilterit/          # data/probfilter: Bloom and Cuckoo filters on Redis Stack — membership, deletes, shared rebuilds
├── auditit/               # data/audit: storage conformance, atomic and idempotent batches on SQL servers
├── brokerit/              # transport/broker: JetStream redelivery, restarts, shutdown, idempotency middleware over Redis
├── outboxit/              # data/outbox: transactional delivery against MongoDB, PostgreSQL, MariaDB and MySQL
├── mongoit/               # data/mongo: transactions abort on panic, migration failure and rollback, keyset paging with FindPage
├── projectionit/          # data/projection: projections accepted by MongoDB and PostgreSQL, keyset paging under a projection
├── sagait/                # data/saga: storage contracts on SQL, Mongo and Redis + orchestrator over SQL
├── schedulerit/           # service/scheduler: end-to-end scenarios + storage contracts on Mongo, Redis and SQL
├── idempotencyit/         # data/idempotency: storage contracts on Redis, Mongo and NATS + Keeper and HTTP middleware
├── cacheit/               # data/cache: the Redis storage and the Cache facade (singleflight, negative caching) on Redis
└── internal/livebackend/  # fixtures shared by several suites: Redis address and client, per-test names and key prefixes
```

## Why a separate module

Exercising the SQL filter translators needs `pgx` and `go-sql-driver/mysql` (`clickhouse-go` is already a root dependency of the ClickHouse audit
storage). Those are test dependencies of a *library*, so putting them in the root `go.mod` would push both into the dependency graph and `go.sum` of
every project that imports go-atlas. A separate module with `replace github.com/altessa-s/go-atlas => ../..` keeps the toolkit's own graph clean, the
same way `devtools/` does.

The cost: these tests are not part of the root module's `go test ./...`. Run them explicitly.

## Running

```bash
docker compose -f tests/integration/docker-compose.yml up -d --wait
make test-integration
docker compose -f tests/integration/docker-compose.yml down -v
```

Ports are deliberately non-default so the stack never collides with a database already running locally. Every test **skips** rather than fails when its
server is unreachable, so `make test-integration` is green on a machine with no Docker — check the skip count if you expect coverage.

| Backend       | Default address           | Environment override                       |
|---------------|---------------------------|--------------------------------------------|
| ClickHouse    | `127.0.0.1:19001`         | `CLICKHOUSE_ADDR`, `CLICKHOUSE_DB`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD` |
| MariaDB       | `127.0.0.1:13306`         | `MARIADB_DSN`                              |
| MySQL         | `127.0.0.1:13307`         | `MYSQL_DSN`                                |
| PostgreSQL    | `127.0.0.1:15432`         | `POSTGRES_DSN`                             |
| MongoDB       | `127.0.0.1:27019` (replica set `rs0`) | `MONGO_PORT`, `MONGO_URI`      |
| Meilisearch   | `http://127.0.0.1:17700`  | `MEILI_URL`, `MEILI_KEY`                   |
| RediSearch    | `127.0.0.1:16379`         | `REDIS_ADDR`                               |
| NATS          | `nats://127.0.0.1:14222`  | `NATS_URL`                                 |

Each test creates its own throwaway table, database or index, named with a timestamp suffix, and drops it on cleanup — two runs against the same server
never collide.

### MongoDB runs as a replica set

The compose service starts `mongod --replSet rs0` and initiates the set from its own healthcheck, because `data/outbox` fetches and locks a batch inside
a transaction and transactions do not exist on a standalone mongod. The service is only reported healthy once it has a primary.

Connect with **`directConnection=true`** — the default `MONGO_URI` already does. A single-node set advertises the address it sees inside its own
container (`127.0.0.1:27017`), which is not the published one, so a driver doing topology discovery from the host follows that advertisement to a port
nothing listens on and stalls until server selection times out.

When host port 27019 is taken (another project's MongoDB), move the stack's port with `MONGO_PORT`: the compose file publishes MongoDB on it and the
suites connect to it. Set it for both steps, e.g. `MONGO_PORT=27029 make integration-up` and `MONGO_PORT=27029 make test-integration`. `MONGO_URI` still
overrides the whole address. The suites that require Mongo explicitly (`sagait`, `idempotencyit`) run their Mongo cases only when `MONGO_URI` is set.

## filterit — the data/filter corpus

One dataset of seven rows and one list of filter expressions run through all six translators. `corpus_test.go` is pure data; each
`*_integration_test.go` is the adapter that loads the dataset into its backend and executes the translated query.

Two matrices:

| Matrix         | Asserts                                                                                     |
|----------------|---------------------------------------------------------------------------------------------|
| `Cases()`      | The expression translates, the **server accepts it**, and it selects exactly the right rows  |
| `Rejections()` | The expression is refused with a named sentinel — `ErrFieldNotAllowed`, `ErrMaxDepthExceeded`, … |

The middle assertion is the one unit tests cannot make. A clause can match a golden string byte for byte and still be a syntax error, name a function
the server does not have, or type-mismatch on the wire.

### Audit regressions

`audit_regression_test.go` executes dynamic SQL operands, Unicode string/array sizes, escaped RediSearch tags, empty membership, and Lua expressions
through Redis `EVAL`. Unsupported dynamic Meilisearch operands must return `ErrUnsupportedOperation` before reaching the server.

The filter suite uses the MariaDB translator for MySQL too. To repeat its corpus and SQL regressions against the compose MySQL service:

```bash
MARIADB_DSN='atlas:atlas@tcp(127.0.0.1:13307)/atlas?parseTime=true&loc=UTC&charset=utf8mb4' go test -C tests/integration -race -count=1 -run 'TestMariaDB|TestRegexAnchors/mariadb|TestLiveSQLAudit/mariadb' ./filterit
```

### Divergence is asserted, not tolerated

Where a backend returns a *different but correct* answer, `Case.Differs` pins it:

| Expression           | Most backends | Diverges                                                                   |
|----------------------|---------------|----------------------------------------------------------------------------|
| `name == "Alice"`    | `{1}`         | MariaDB, Meilisearch, RediSearch → `{1,5}` (case folding)                  |
| `name > "M"`         | `{5,7}`       | MariaDB, PostgreSQL → `{7}` (collation orders `alice` below `M`)           |
| `name.contains("Al")`| `{1}`         | MariaDB, Meilisearch, RediSearch → `{1,5}`                                 |

Where the case does not apply at all, `Case.Skip` records the reason. Run with `-v` to read them:

```bash
go test -C tests/integration -v ./filterit/ 2>&1 | grep -A1 SKIP
```

### Translator bugs this found

Three bugs surfaced on the first full run. None was caught by the unit tests, which assert on generated strings and never assemble a whole query against
real data. All three are fixed; the cases that exposed them are now ordinary corpus entries.

| Bug                                                                                              | Backends                         | Fix                                                        |
|--------------------------------------------------------------------------------------------------|----------------------------------|------------------------------------------------------------|
| A bare boolean identifier (`active`) was not rendered as a boolean test                           | MongoDB, Meilisearch, RediSearch | `acceptPredicate` at the root and on both logical operands  |
| `in` ignored the field schema and always emitted TAG syntax, so `role in [2,3]` matched nothing   | RediSearch                       | `buildIn` dispatches on `FieldType`                        |
| `field != null` matched every document, including those without the attribute                     | Meilisearch                      | paired with the matching `EXISTS` / `NOT EXISTS` check      |

The first was an asymmetry rather than a design choice: all three translators already special-cased `!active`, and only the positive form was missing.
MongoDB failed loudly with `ErrInvalidExpression`; Meilisearch emitted a bare attribute name the server rejected as a missing operator; RediSearch
emitted it as a free-text term, so the query ran and matched whatever the TEXT fields happened to contain.

The third was the one worth finding. `deletedAt != null` is the shape of a soft-delete filter, and against Meilisearch it silently returned the deleted
documents. `TestMeili_NullFilterSemantics` is its regression guard and still asserts what the bare `IS NOT NULL` would have returned, so the reason the
paired form exists stays visible.

Three more came from running the scheduler storage contracts against live backends. RediSearch rendered `startsWith`/`contains` in TEXT syntax whatever
the field type, so on a TAG field they matched nothing; they now take the tag form (`@status:{ar*}`), pinned by the "startsWith on a keyword field"
case. RediSearch also rendered `a || b` as `(a)|(b)` without grouping the union, and since an intersection binds tighter, `x && (a || b)` ran as
`(x a)|(b)`; the union is now grouped whole, pinned by the "or grouped inside and" and "negated conjunction containing or" cases (the stale RediSearch
skip on "not" went with it — a negated range works). MongoDB translated `size()` to the array-only `$size`, so a string never matched; a string is now
measured in code points with `$strLenCP`, the Mongo skips on the string-size cases are gone, and `TestMongo_SizeSemantics` pins every field shape
(absent, null, arrays, strings, other values).

## leadelectit — leader election against a live broker

The unit tests for `data/leadelect` run against an embedded NATS server and assert on the pieces in isolation. Three properties only exist once several
electors compete for one key through a real broker over real time, and those are what this package covers.

| Scenario                                   | Asserts                                                                                       |
|--------------------------------------------|-----------------------------------------------------------------------------------------------|
| `SingleLeaderAmongPeers`                   | Five electors, many renewals: never two claims at once, and leadership does not drift          |
| `FenceAdvancesWithRenewals`                | The fencing token moves with each renewal, so a replayed write is distinguishable from a fresh one |
| `HandoverOnGracefulStop`                   | A resign releases the key immediately — the successor does not wait for expiry                  |
| `FailoverAfterAbruptLoss`                  | A holder that dies without resigning still releases the lease, via server-side key expiry       |
| `PartitionedLeaderSelfDemotes`             | A node cut off from the broker stops claiming leadership unprompted, and its token drops to `0` |
| `CallbacksFireOnTransitions`               | The became-leader callback fires on the elected node, then on its successor                     |
| `FenceMonotonicAcrossRepeatedFailovers`    | The token never stalls or moves backwards across a chain of handovers                           |
| `Bucket_TTLReconciledOnAdoption`           | A pre-existing bucket without a TTL is refused, and migrated only with `WithMigrateBucketTTL`   |
| `Bucket_KeyExpiresWithoutRenewal`          | The server ages out an unrenewed election key — the mechanism every failover above rests on     |

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

The SQL provider runs the contract suite from [`data/locks/dlock/providers/providertest`](../../data/locks/dlock/providers/providertest) on PostgreSQL,
MariaDB and MySQL — each contract over its own table, with leases aged out by writing `expires_at` directly — and the same critical-section recording
with several `dlock` instances racing for one key.

| Scenario                                  | Asserts                                                                                    |
|-------------------------------------------|--------------------------------------------------------------------------------------------|
| `ContendersNeverOverlap`                  | Six holders, three rounds each, holding across several renewals: peak concurrency stays one |
| `Synchronize_SerializesACounter`          | A read-modify-write from five processes loses no updates                                    |
| `HeldLockRefusesOthers`                   | A second acquisition is refused, and refused immediately                                    |
| `SurvivesLongerThanTheAcquireTimeout`     | The acquire timeout bounds waiting, not holding                                             |
| `AbandonedLockExpires`                    | A holder that dies without releasing still frees the key, via server-side expiry            |
| `FencingTokenAdvancesAcrossHolders`       | Each successive holder sees a strictly higher token                                         |
| `ReleaseLeavesNoKey`                      | A released lock is gone at once — the next contender does not wait out the TTL              |
| `Bucket_KeyTTLFollowsTheLockTTL`          | The bucket's key TTL is the configured lock TTL, not a constant                             |

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

| Scenario                              | Asserts                                                                                         |
|---------------------------------------|-------------------------------------------------------------------------------------------------|
| `ExactlyLimitPerWindow`               | `limit` admissions, `Remaining` counting down, `Reset` = oldest request + period, the public sentinel |
| `WindowSlidesPerRequest`              | Each request leaves the window on its own; a rejection occupies no slot                         |
| `ResetClearsTheWindow`                | `Reset` gives the whole limit back; resetting an unknown key is not an error                    |
| `KeysAreIsolated`                     | An exhausted key limits no other                                                                |
| `ConcurrentCallersNeverExceedLimit`   | 32 goroutines on one key admit at most `limit`, and the window then holds exactly `limit`       |
| `ReplicasShareOneLimit`               | The same race from two provider instances on their own connections                              |

On top of each storage, `TestTokenBucket_RulesOverSharedStorage` drives IP, CIDR and token rules from two replicas, and `TestBudget_*` spends one budget
from two replicas, sees it come back a period later, and reports an unreachable Redis as an error rather than an exhausted budget.

**Contention costs the NATS provider latency, not correctness.** Its compare-and-swap loop retries a revision conflict after a randomized backoff, up to
`WithMaxCASAttempts` attempts (64 by default), so under the 32-goroutine race and across two replicas every call still ends in a decision: the suite
requires zero errors, and the limit is never exceeded. A call that exhausts its attempts returns `ErrContention`.

## probfilterit — Bloom and Cuckoo filters on Redis Stack

miniredis has no RedisBloom commands, so the unit tests of the Redis storages stop at structure. Here every path that touches a filter runs against
Redis Stack: the reserve script, the journaled insert, the staging reserve, the journal drain, the commit with its journal replay, the rebuild lease and
the generation-fenced delete — and the RedisBloom error texts the storages recognize, such as the "not found" of a `NOCREATE` insert.

| Scenario                                    | Asserts                                                                                   |
|---------------------------------------------|-------------------------------------------------------------------------------------------|
| `Bloom_InstancesShareOneFilter`             | Two processes see each other's inserts; the filter is reserved with the configured capacity |
| `Bloom_BatchLargerThanOneCommand`           | A batch split across several `BF.INSERT` commands loses no value                          |
| `Bloom_FalsePositiveRate`                   | Measured over 20 000 values never added, at the expected size                              |
| `Bloom_RebuildReplacesContents`             | Streamed and collected loads both swap atomically and leave no staging key, lease or journal |
| `Bloom_FailedRebuildKeepsContents`          | A source failing midway publishes nothing and frees the lease                             |
| `Bloom_InsertsDuringRebuildSurvive`         | Own, peer (a burst larger than one drain batch) and through-the-commit inserts all survive |
| `Bloom_ConcurrentRebuildIsRefused`          | A second rebuild fails fast with `ErrRebuildInProgress`; the staging key carries a TTL until promoted |
| `Bloom_SupersededRebuildDoesNotPublish`     | A rebuild whose lease lapsed is refused by the journal drain; its successor publishes      |
| `Bloom_RecreatedFilterIsNotRebuilt`         | A deleted live key is recreated with its parameters, and no longer reports a committed rebuild |
| `Bloom_CloseAbortsRunningRebuild`           | `Close` interrupts a rebuild, which leaves nothing behind                                 |
| `Cuckoo_InstancesShareAddsAndDeletes`       | Inserts and deletes by one process are what another sees; stats count slots, not bytes    |
| `Cuckoo_DuplicatesNeedOneDeleteEach`        | A value inserted twice stays present until deleted twice                                  |
| `Cuckoo_MissingFilter`                      | Delete, lookup and stats of an uncreated filter are answers, not errors, and create nothing |
| `Cuckoo_RebuildReplacesContents`            | A snapshot above the capacity fits the staging filter; deletes work across the generation bump |
| `Cuckoo_InsertsDuringRebuild`               | Own inserts are replayed; a peer's are dropped, as documented                             |
| `Cuckoo_SupersededRebuildDoesNotPublish`    | The commit script itself refuses a rebuild whose lease lapsed                             |
| `Factory_BuildsRedisFilters`                | The factory checks the live eviction policy and rebuilds on start                         |

The superseded scenarios delete the rebuild lease key to stand in for its expiry, so they name it with the metadata key layout of
`data/probfilter/internal/redisfilter`; a change to that layout has to be mirrored in `harness_test.go`.

## outboxit — transactional delivery against live MongoDB and SQL servers

The unit tests for `data/outbox` run against in-memory stores and assert on the state the outbox *decided* to write. Whether that state survives a real
store is a different question: the batch fetch takes its lock inside a transaction, every deadline is evaluated against the server clock, and writes are
fenced by a lock token. None of those mechanisms exists until a real MongoDB is on the other side.

The `Dispatch_*`, `Retry_*`, `Concurrency_*`, `Lifecycle_*` and `Store_*` scenarios run on every outbox store — MongoDB and the SQL store on PostgreSQL,
MariaDB and MySQL — each in its own subtest over its own tables (`forEachBackend`). The MySQL DSN deliberately sets `loc=Asia/Tokyo` with
`parseTime=true`: the SQL store must not let the driver shift an instant. The `Watch_*` and `Dedup_*` scenarios are MongoDB-only (change streams; NATS
de-duplication through the broker adapter).

| Scenario                                          | Asserts                                                                                     |
|---------------------------------------------------|---------------------------------------------------------------------------------------------|
| `Dispatch_DeliversSavedEvent`                     | The baseline: saved, delivered, marked sent, lease released                                  |
| `Dispatch_CommittedTransactionPublishesExactlyOnce` | Business write and event commit together, and the event is published once                  |
| `Dispatch_AbortedTransactionPublishesNothing`     | A rolled-back write leaves no event — the dual-write gap the pattern exists to close         |
| `Dispatch_RejectedSaveLeavesTheTransactionCommittable` | Validation runs before any store write, so a rejected batch does not force an abort      |
| `Dispatch_CompactionPublishesOnlyTheLatestPerKey` | Superseded events are recorded as skipped, not silently dropped                              |
| `Dispatch_SelectsOldestEventsFirstAcrossBatches`  | Each cycle selects the oldest waiting events, the ordering compaction depends on              |
| `Retry_FailedAttemptIsPersisted`                  | A failed attempt reaches the store: attempt counted, error readable, backoff stamped         |
| `Retry_BackoffWithholdsTheEventUntilItElapses`    | The retry deadline is enforced by the server and survives a restart                          |
| `Retry_DeadLettersWhenTheBudgetRunsOut`           | One attempt per cycle, then terminal — and never picked up again                             |
| `Retry_PermanentFailureIsRejectedWithoutRetrying` | A permanent error costs one attempt, not the whole budget                                    |
| `Retry_SucceedsOnceTheDestinationRecovers`        | The retry reuses the event ID, so a broker can deduplicate it                                 |
| `Concurrency_EachEventIsDeliveredOnce`            | Four dispatchers over 60 events: no event handed to two handlers                             |
| `Concurrency_ReclaimedLeaseCannotOverwriteTheNewerResult` | A revoked lease cannot resurrect an event another worker already delivered           |
| `Concurrency_SweeperLeavesLiveLeasesAlone`        | An in-flight delivery is not reclaimed, so duplicates stay exceptional                       |
| `Concurrency_OverlappingCyclesOnOneInstanceCollapse` | A cycle firing while another is in flight is a no-op                                       |
| `Lifecycle_ExpiredEventsAreNeverDispatched`       | A deadline that has passed suppresses delivery and is recorded                               |
| `Lifecycle_CleanupKeepsDeadLetteredEvents`        | Retention sweeps successes and keeps what an operator still has to look at                   |
| `Lifecycle_StatsReportBacklogDeadLettersAndLag`   | The gauges alerting depends on, computed against the server clock                            |
| `Store_PayloadsRoundTrip`                         | `nil`, empty and binary payloads come back exactly as saved                                  |
| `Store_LargeKeysAndErrors`                        | Keys and handler errors over 64 KB persist; a huge error does not roll back a sibling result  |
| `Store_ErrorsWithNULPersist`                      | A handler error containing NUL persists without rolling back a sibling result                 |
| `Store_InstantsAreNotShifted`                     | Client instants round-trip unshifted under any driver time zone; backlog age on the server clock |
| `Store_SQLWatchIsUnsupported`                     | A SQL store reports `ErrWatchUnsupported`; dispatch stays on the poll schedule               |
| `Watch_SupportsChangeStreamsOnAReplicaSet`        | The capability probe reads a real `hello` reply the way it expects to                        |
| `Watch_DeliversWithoutAPollCycle`                 | A change stream alone carries a saved event to its handler — no schedule, no manual cycle    |
| `Watch_DeliversNothingUntilTheTransactionCommits` | The stream reports an insert at commit, so a notification can never publish an aborted write |
| `Watch_DrainsBacklogLargerThanOneBatch`           | A full batch re-arms the drain; the remainder does not wait for the next tick                |
| `Watch_CoexistsWithPollingDispatchers`            | A watcher and a poller racing one collection still deliver each event once                   |

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

| Bug                                                                                                     | Guard                                                    |
|---------------------------------------------------------------------------------------------------------|----------------------------------------------------------|
| A failed dispatch was never written back — the concurrent batch helper drops the result of any item whose function returned an error, so attempts never grew and no event could reach a terminal status | `Retry_FailedAttemptIsPersisted`                         |
| A dispatcher whose lock had been reclaimed could overwrite the result of the worker that took the event over | `Concurrency_ReclaimedLeaseCannotOverwriteTheNewerResult` |
| Retention compared a server-clock deadline against a **client-stamped** `published_at`, so a host whose clock ran ahead of the database kept events past their window — and one running behind deleted them early | `Lifecycle_CleanupKeepsDeadLetteredEvents`               |

The third surfaced only because the Docker VM's clock sat ~40 ms behind the host's. That is the ordinary condition on a developer machine, and a badly
synced production host is off by far more; the store now stamps `published_at` with `$$NOW`, the same clock the sweep compares against.

## mongoit — data/mongo against a live replica set

`mongo_integration_test.go` checks that a transaction whose handler panics is aborted rather than committed, that a failing migration keeps the
migrations already applied unless a full rollback is requested, and that concurrent clients each migrate their own database. The test migrations
register in the global mongo-migrate registry, as an application's do; `1_marker_test.go` and `2_gate_test.go` carry the versions in their names.

`keyset_integration_test.go` pages through `FindPage` with [`data/keyset`](../../data/keyset) tokens: ties broken by `_id` values of several BSON types,
tokens refused under a changed filter, sort or subject, a document missing the sort value (`ErrKeysetMissingKey`) and paging under a projection.

## auditit — audit storage against live SQL servers

The conformance suite from [`data/audit/storages/storagetest`](../../data/audit/storages/storagetest) — `(timestamp millisecond, ID)` order, page
boundaries, continuation under inserts, `Count`, time ranges, replays and page-token bindings — runs against the SQL storage on PostgreSQL, MariaDB and
MySQL, each contract over its own table. A batch split into several INSERTs is replayed without duplicating events, and a batch whose last INSERT the
server rejects leaves no row behind.

## schedulerit — the scheduler against live storages

Every scenario (dispatch round trip, at-most-once across instances, crash recovery, leadership gating, failure recording) and the storage contract suite
from [`service/scheduler/storages/storagetest`](../../service/scheduler/storages/storagetest) — each contract over its own throwaway storage — run
against each storage: MongoDB, Redis Stack, PostgreSQL, MariaDB and MySQL. The SQL backends additionally run the SQL-only semantics: non-ASCII
`endsWith`/`size()` and history errors larger than 64 KB.

The MySQL service starts with `latin1` as its default character set on purpose: the SQL storage declares its own `utf8mb4` NO PAD collations, and the
suite proves it never inherits the database defaults.

## sagait — saga storages against live servers

The storage contract suite from [`data/saga/storages/storagetest`](../../data/saga/storages/storagetest) — insert-if-absent creation, the version
compare-and-swap under concurrent writers, exact ID identity and the recovery predicate at full lease precision — runs against the SQL store on
PostgreSQL, MariaDB and MySQL and against the MongoDB and Redis stores, each contract over its own throwaway table, database or key prefix. An
orchestrator built through the factory, which creates the schema, then runs a compensating saga over each SQL server. The MongoDB contracts run only
with `MONGO_URI` set (`mongodb://127.0.0.1:27019/?directConnection=true` for the compose stack), so the suite never touches another project's MongoDB on
that port.

## idempotencyit — idempotency storages against live servers

One contract suite runs on the Redis, MongoDB and NATS JetStream storages, each test over its own key prefix, database or bucket. The MongoDB backend
runs only with `MONGO_URI` set, as in `sagait`. The Redis storage fences `Complete`, `Steal` and `Release` with Lua scripts comparing the stored bytes;
every branch of both scripts — match, mismatch, missing key — runs against real Redis, with values that contain NUL bytes and are not valid UTF-8, since
that is where miniredis and Redis can disagree.

| Scenario                                  | Asserts                                                                                      |
|-------------------------------------------|----------------------------------------------------------------------------------------------|
| `Storage_AttemptLock`                     | The first caller acquires with a token; the next sees the holder's exact bytes and no token  |
| `Storage_AcceptsTheKeysTheKeeperBuilds`   | Nested v2 keys of every layer work, each on its own entry, also for colliding `:`-joined tuples |
| `Storage_NATSRefusesKeysEscaped…`         | NATS stores a key escaped to exactly 3072 bytes; one byte more fails with `ErrKeyTooLong`      |
| `Storage_Complete`                        | Holder token writes; a foreign or stale token and a vanished key are refused without writing |
| `Storage_Steal`                           | A matching expectation takes over and fences the old holder; a stale one or a missing key fails |
| `Storage_Release`                         | Only the holder's token deletes; a completed result survives its holder's release             |
| `Storage_PerCallTTLExpiresOnTheServer`    | `AttemptLockWithTTL` ages the key out on the server; the expired holder is refused afterwards |
| `Storage_ConfiguredTTLExpiresOnTheServer` | The storage TTL ages out a lock and a completed result, whatever the lock's own TTL was       |
| `Storage_Concurrent*HasOneWinner`         | 32 racing `AttemptLock`, `Steal` or `Release` calls: exactly one wins                         |
| `Storage_LargeBinaryValues`               | The lifecycle with 512 KiB random values; a difference in the last byte is detected           |
| `Redis_*`                                 | Lua writes renew the key with the storage TTL; refused or missing-key scripts write nothing   |
| `Keeper_*`                                | Replay of a completed result, release and retry, orphan reclaim under contention, fingerprints, namespaces |
| `Middleware_*`                            | The HTTP middleware end to end: replay refused, concurrent duplicate gets 409, failure retried |

The HTTP middleware and the gRPC interceptor compose `("transport", service, key)` and the broker wrapper `("ibk", consumer, key)` in the
length-prefixed v2 format of [`data/idempotency.ComposeKey`](../../data/idempotency/README.md#storage-key-format), and the Keeper wraps every key in
`("namespace", namespace, key)` — `idk:v2:9:namespace0:38:idk:v2:9:transport12:POST:/orders3:123`. NATS KV keys allow only `[-/_=.a-zA-Z0-9]`, so the
NATS storage escapes the final key once (`:` becomes `=3A`) and refuses one longer than 3072 bytes after escaping.

## cacheit — the cache against a live Redis

The Redis cache storage needs no Lua — each operation is one command or a `MULTI`/`EXEC` pipeline — so the suite is about expiry on the server clock,
large values and what the `Cache` facade leaves in Redis: its default TTL, `NoTTL`, the fallback's own TTL, one fallback run for many concurrent callers
of `GetWithFallback`, negative entries that answer without the fallback until they expire, and per-tenant keys.

## brokerit — transport/broker against a live JetStream

The unit tests for `transport/broker` drive the NATS provider through fakes. What a broker promises — redelivery, terminal outcomes, durable progress
across restarts — is decided by the server, so these scenarios run each verdict a handler can give through a real JetStream, every test on a memory
stream of its own. The idempotency scenarios keep their records in Redis under a key prefix of their own.

| Scenario                                              | Asserts                                                                                        |
|-------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `Delivery_RoundTripCarriesDataAndHeaders`             | Payload, subject, application headers and the creation stamp arrive; no `Nats-*` header leaks   |
| `Delivery_BatchArrivesOnceAndSettles`                 | A pipelined batch is delivered once per message and leaves nothing pending                     |
| `Delivery_NakWithBackOffWaitsTheScheduledDelay`       | Each redelivery waits the delay the schedule gives its delivery count, not the ack wait        |
| `Delivery_NakRedeliversImmediately`                   | A plain `Nak` does not wait for the ack timer                                                  |
| `Delivery_TermStopsRedeliveryAndReportsTheReason`     | A terminated message never comes back; the reason reaches the terminated advisory             |
| `Delivery_MaxDeliverEndsRedeliveryWithAnAdvisory`     | Exactly `MaxDeliver` deliveries, then the max-deliveries advisory — the only dead-letter hook   |
| `Delivery_UnackedMessageIsRedeliveredAfterTheAckWait` | No verdict means redelivery after the ack wait, which the message reports as `AckTimeout`      |
| `Delivery_InProgressHeartbeatsHoldOffRedelivery`      | A handler holding a message for three ack waits keeps it through `inprogress.Manager`          |
| `Delivery_PanicIsRedeliveredAndTheSubscriptionSurvives` | A recovered panic leaves the message for redelivery and the subscription consuming              |
| `Restart_NothingLostWhenAConsumerDiesMidStream`       | A successor on the same durable consumer receives everything an abruptly lost instance did not ack |
| `Restart_GracefulHandoverLosesAndDuplicatesNothing`   | `Unsubscribe` mid-stream drains and acks; the successor takes the rest, each message once      |
| `Shutdown_UnsubscribeWaitsForTheInFlightHandler`      | `Unsubscribe` cancels the handler context, returns only after the handler, and its ack lands   |
| `Shutdown_AbandonedInFlightMessageGoesToTheNextInstance` | A handler that gives up on cancellation leaves its message to the next instance             |
| `Shutdown_SubscriberLifecycleLeaksNoGoroutines`       | Repeated subscribe/unsubscribe cycles return the goroutine count to its baseline (serial test) |
| `Idempotency_DuplicatesAreProcessedOnce`              | Copies of one message ID run the handler once and are acknowledged without redelivery          |
| `Idempotency_PanicReleasesTheKeyForTheRedelivery`     | A panicking handler's key is released, so the first redelivery is processed                    |
| `Idempotency_ConcurrentDuplicatesAcrossTwoInstancesRunOnce` | Two instances racing for copies of one ID: one side effect per ID, the loser backs off  |
| `Idempotency_DeduplicateIDReachesTheSubscriber`       | The deduplication ID survives the wire as `deduplicate_id`, so `DefaultKeyFunc` keys on it     |
| `Idempotency_NATSKVStorageAcceptsTheMiddlewareKeys`   | The NATS KV storage locks and completes the escaped v2 key the middleware builds               |
| `Idempotency_ConsumerScopesDoNotCollide`              | Consumers scoped `a:b` and `a` both process messages `c` and `b:c`: no record is shared         |

The NATS provider maps `deduplicate_id` onto `Nats-Msg-Id` on publish and back onto `deduplicate_id` on receive, so the key the broker outbox sets on
every event reaches a consumer. The broker wrapper composes `("ibk", consumer, key)` in the v2 key format, which NATS KV stores escaped, so a `:` in a
consumer scope or a message ID can neither break the NATS KV storage nor make two consumers share a record.

The broker outbox's own NATS path — the event ID reused as `Nats-Msg-Id`, a republish collapsed by JetStream — is covered by `outboxit` (`Dedup_*`); its
storages are MongoDB and SQL, so it has no NATS-only path to exercise here.

## Adding a backend

Implement the four-method `backend` interface in `harness_test.go` — `name`, `setup`, `search`, `translate` — and call `runCorpus` and `runRejections`
from a `Test<Name>` pair. `setup` must `Skipf` when the server is unreachable rather than failing.
