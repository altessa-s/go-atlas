# tests/integration

Integration suite for `go-atlas`, kept in its own Go module.

```
tests/integration/
├── go.mod                 # separate module — see below
├── docker-compose.yml     # the backends the suite runs against
├── filterit/              # data/filter: shared corpus + one adapter per backend
├── leadelectit/           # data/leadelect: multi-node election against a live broker
├── dlockit/               # data/locks/dlock: contended locking against a live broker
└── outboxit/              # data/outbox: transactional delivery against a live MongoDB
```

## Why a separate module

Exercising the SQL filter translators needs `clickhouse-go`, `pgx` and `go-sql-driver/mysql`. Those are test dependencies of a *library*, so
putting them in the root `go.mod` would push all three into the dependency graph and `go.sum` of every project that imports go-atlas. A separate
module with `replace github.com/altessa-s/go-atlas => ../..` keeps the toolkit's own graph clean, the same way `devtools/` does.

The cost: these tests are not part of the root module's `go test ./...`. Run them explicitly.

## Running

```bash
docker compose -f tests/integration/docker-compose.yml up -d --wait
make test-integration
docker compose -f tests/integration/docker-compose.yml down -v
```

Ports are deliberately non-default so the stack never collides with a database already running locally. Every test **skips** rather than fails
when its server is unreachable, so `make test-integration` is green on a machine with no Docker — check the skip count if you expect coverage.

| Backend       | Default address           | Environment override                       |
|---------------|---------------------------|--------------------------------------------|
| ClickHouse    | `127.0.0.1:19001`         | `CLICKHOUSE_ADDR`, `CLICKHOUSE_DB`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD` |
| MariaDB       | `127.0.0.1:13306`         | `MARIADB_DSN`                              |
| PostgreSQL    | `127.0.0.1:15432`         | `POSTGRES_DSN`                             |
| MongoDB       | `127.0.0.1:27019` (replica set `rs0`) | `MONGO_URI`                    |
| Meilisearch   | `http://127.0.0.1:17700`  | `MEILI_URL`, `MEILI_KEY`                   |
| RediSearch    | `127.0.0.1:16379`         | `REDIS_ADDR`                               |
| NATS          | `nats://127.0.0.1:14222`  | `NATS_URL`                                 |

Each test creates its own throwaway table, database or index, named with a timestamp suffix, and drops it on cleanup — two runs against the same
server never collide.

### MongoDB runs as a replica set

The compose service starts `mongod --replSet rs0` and initiates the set from its own healthcheck, because `data/outbox` fetches and locks a batch
inside a transaction and transactions do not exist on a standalone mongod. The service is only reported healthy once it has a primary.

Connect with **`directConnection=true`** — the default `MONGO_URI` already does. A single-node set advertises the address it sees inside its own
container (`127.0.0.1:27017`), which is not the published one, so a driver doing topology discovery from the host follows that advertisement to a
port nothing listens on and stalls until server selection times out.

## filterit — the data/filter corpus

One dataset of seven rows and one list of filter expressions run through all six translators. `corpus.go` is pure data; each
`*_integration_test.go` is the adapter that loads the dataset into its backend and executes the translated query.

Two matrices:

| Matrix         | Asserts                                                                                     |
|----------------|---------------------------------------------------------------------------------------------|
| `Cases()`      | The expression translates, the **server accepts it**, and it selects exactly the right rows  |
| `Rejections()` | The expression is refused with a named sentinel — `ErrFieldNotAllowed`, `ErrMaxDepthExceeded`, … |

The middle assertion is the one unit tests cannot make. A clause can match a golden string byte for byte and still be a syntax error, name a
function the server does not have, or type-mismatch on the wire.

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

Three bugs surfaced on the first full run. None was caught by the unit tests, which assert on generated strings and never assemble a whole
query against real data. All three are fixed; the cases that exposed them are now ordinary corpus entries.

| Bug                                                                                              | Backends                         | Fix                                                        |
|--------------------------------------------------------------------------------------------------|----------------------------------|------------------------------------------------------------|
| A bare boolean identifier (`active`) was not rendered as a boolean test                           | MongoDB, Meilisearch, RediSearch | `acceptPredicate` at the root and on both logical operands  |
| `in` ignored the field schema and always emitted TAG syntax, so `role in [2,3]` matched nothing   | RediSearch                       | `buildIn` dispatches on `FieldType`                        |
| `field != null` matched every document, including those without the attribute                     | Meilisearch                      | paired with the matching `EXISTS` / `NOT EXISTS` check      |

The first was an asymmetry rather than a design choice: all three translators already special-cased `!active`, and only the positive form was
missing. MongoDB failed loudly with `ErrInvalidExpression`; Meilisearch emitted a bare attribute name the server rejected as a missing operator;
RediSearch emitted it as a free-text term, so the query ran and matched whatever the TEXT fields happened to contain.

The third was the one worth finding. `deletedAt != null` is the shape of a soft-delete filter, and against Meilisearch it silently returned the
deleted documents. `TestMeili_NullFilterSemantics` is its regression guard and still asserts what the bare `IS NOT NULL` would have returned, so
the reason the paired form exists stays visible.

## leadelectit — leader election against a live broker

The unit tests for `data/leadelect` run against an embedded NATS server and assert on the pieces in isolation. Three properties only exist once
several electors compete for one key through a real broker over real time, and those are what this package covers.

| Scenario                                   | Asserts                                                                                       |
|--------------------------------------------|-----------------------------------------------------------------------------------------------|
| `SingleLeaderAmongPeers`                   | Five electors, many renewals: never two claims at once, and leadership does not drift          |
| `FenceAdvancesWithRenewals`                | The fencing token moves with each renewal, so a replayed write is distinguishable from a fresh one |
| `HandoverOnGracefulStop`                   | A resign releases the key immediately — the successor does not wait for expiry                  |
| `FailoverAfterAbruptLoss`                  | A holder that dies without resigning still releases the lease, via server-side key expiry       |
| `PartitionedLeaderSelfDemotes`             | A node cut off from the broker stops claiming leadership unprompted, and its token drops to `0` |
| `CallbacksFireOnTransitions`               | The became-leader callback fires on the elected node, then on its successor                     |
| `FenceMonotonicAcrossRepeatedFailovers`    | The token never stalls or moves backwards across a chain of handovers                           |
| `Bucket_TTLReconciledOnAdoption`           | Adopting a bucket that predates the provider does not leave leases that never expire            |
| `Bucket_KeyExpiresWithoutRenewal`          | The server ages out an unrenewed election key — the mechanism every failover above rests on     |

**Sampling shows overlap, it cannot rule it out.** `Observer` polls every 25 ms, well under the 2 s lease. A round with two claimants proves mutual
exclusion broke; a clean run is evidence it held, not proof — an overlap shorter than the interval goes unseen. Where a handover completes in tens
of milliseconds the scenario holds each term briefly so the poller has something to record, and reads the fencing tokens directly rather than
trusting the recording to have caught every term.

**Two different durations govern failover.** The election TTL (`WithTTL`) bounds how long a node keeps believing it leads once it can no longer
renew. The bucket's key TTL — fixed by the provider, not derived from the election TTL — is what releases the key when a holder dies without
resigning. A failover after an abrupt loss is therefore bounded by the latter, which is why `FailoverAfterAbruptLoss` takes about ten seconds while
`HandoverOnGracefulStop` takes milliseconds.

## dlockit — contended locking against a live broker

A lock that is never contested is indistinguishable from no lock at all. These scenarios put several holders on one key through a real broker and
check that their critical sections never coincide.

| Scenario                                  | Asserts                                                                                    |
|-------------------------------------------|--------------------------------------------------------------------------------------------|
| `ContendersNeverOverlap`                  | Six holders, three rounds each, holding across several renewals: peak concurrency stays one |
| `SynchronizeSerializesACounter`           | A read-modify-write from five processes loses no updates                                    |
| `HeldLockRefusesOthers`                   | A second acquisition is refused, and refused immediately                                    |
| `SurvivesLongerThanTheAcquireTimeout`     | The acquire timeout bounds waiting, not holding                                             |
| `AbandonedLockExpires`                    | A holder that dies without releasing still frees the key, via server-side expiry            |
| `FencingTokenAdvancesAcrossHolders`       | Each successive holder sees a strictly higher token                                         |
| `ReleaseLeavesNoKey`                      | A released lock is gone at once — the next contender does not wait out the TTL              |
| `Bucket_KeyTTLFollowsTheLockTTL`          | The bucket's key TTL is the configured lock TTL, not a constant                             |

**The recording proves overlap, unlike sampling.** `Critical` is not a poller: every holder reports the instant it entered and the instant it left,
so an overlap of any duration is caught rather than merely likely to be caught. Timestamps come from one process and one clock, which is what makes
comparing them sound.

**Acquisition fails fast.** The NATS provider makes a single attempt and returns `ErrLockNotHeld` when the key is taken; it does not wait for the
holder to finish. Scenarios that need to be serialized rather than rejected loop over `Synchronize` themselves.

**The context scopes the lock, not the call.** Cancel it and the lease stops being renewed and is released, so it must not end before the work the
lock guards. The acquisition attempt is bounded separately, by the provider's `WithAcquireTimeout` — folding that bound into the same context would
release the lock the moment it elapsed.

## outboxit — transactional delivery against a live MongoDB

The unit tests for `data/outbox` run against in-memory stores and assert on the state the outbox *decided* to write. Whether that state survives a
real store is a different question: the batch fetch takes its lock inside a transaction, every deadline is evaluated against the server clock, and
writes are fenced by a lock token. None of those mechanisms exists until a real MongoDB is on the other side.

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

**The recorder proves duplicates, it does not sample for them.** `Recorder` captures every handler invocation with its event ID, so a duplicate is
two entries naming the same event rather than a count that came out high. `Duplicates()` names them and `Timeline()` prints the whole recording
into the failure message.

**Cycles are driven explicitly, never by a scheduler.** A scenario that asserts "nothing was delivered yet" cannot share a store with a background
poller that might deliver it at any moment.

**Three bugs this found.** All were invisible to the unit tests, which inspect the event the outbox built rather than the document that was stored:

| Bug                                                                                                     | Guard                                                    |
|---------------------------------------------------------------------------------------------------------|----------------------------------------------------------|
| A failed dispatch was never written back — the concurrent batch helper drops the result of any item whose function returned an error, so attempts never grew and no event could reach a terminal status | `Retry_FailedAttemptIsPersisted`                         |
| A dispatcher whose lock had been reclaimed could overwrite the result of the worker that took the event over | `Concurrency_ReclaimedLeaseCannotOverwriteTheNewerResult` |
| Retention compared a server-clock deadline against a **client-stamped** `published_at`, so a host whose clock ran ahead of the database kept events past their window — and one running behind deleted them early | `Lifecycle_CleanupKeepsDeadLetteredEvents`               |

The third surfaced only because the Docker VM's clock sat ~40 ms behind the host's. That is the ordinary condition on a developer machine, and a
badly synced production host is off by far more; the store now stamps `published_at` with `$$NOW`, the same clock the sweep compares against.

## Adding a backend

Implement the four-method `backend` interface in `harness_test.go` — `name`, `setup`, `search`, `translate` — and call `runCorpus` and
`runRejections` from a `Test<Name>` pair. `setup` must `Skipf` when the server is unreachable rather than failing.
