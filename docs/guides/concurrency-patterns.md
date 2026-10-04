# Concurrency and Consistency Patterns

- **Status:** Reference. Sections 8 and 9 are normative.
- **Audience:** Backend engineers, architects.
- **Version:** 1.0
- **Related:** [Message Brokers and Queues](message-brokers.md), [Observability](observability.md), [Performance and
  Profiling](performance-profiling.md), [Coordination & Consistency](../coordination.md)

---

## 0. About this guide

A note on terminology first, because it causes confusion. Strictly speaking, the outbox is not a concurrency pattern but a consistency pattern: it does
not address concurrent access, it addresses two systems that cannot be changed atomically. In practice, though, these topics come up in the same
conversation — "two instances at the same time", "the message was delivered twice", "someone overwrote my changes" — so they are covered together here.

The guide is organized by level, because the level determines the tool:

1. **Within a process.** Goroutines, mutexes, channels. Language-level tools.
2. **Multiple instances, one database.** Database locks and constraints. These are sufficient more often than is commonly assumed.
3. **Distributed level.** Different services, different stores, the network. This is where guarantees are weakest; stay away from it until the problem
   forces you there.
4. **Consistency between writing and publishing.** Outbox, idempotency, sagas.
5. **Overload protection.** Timeouts, retries, circuit breakers.

One rule to keep in mind while reading: **move up to the next level only when the previous one cannot do the job.** In our experience, about half of the
distributed locks seen in production could have been replaced with a unique index.

---

## 1. Within a process

### Mutex or channel

The debate is old; the answer is simple. A mutex protects state: there is a shared structure that several goroutines access. A channel transfers
ownership: a value moves from one goroutine to another, and the sender no longer touches it.

If you find yourself implementing a queue with a mutex and a slice, you need a channel. If you pass a pointer to a struct through a channel and both
sides then modify it, you need a mutex — the channel only hides the race.

`sync.RWMutex` makes sense when the workload is heavily skewed toward reads and critical sections are reasonably long. For short critical sections it is
slower than a plain `Mutex` because its synchronization is more expensive.

### Bounded parallelism

An unbounded `go func()` in a loop is the most common reason a service takes down its database. The standard fix:

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(16)

for _, item := range items {
    g.Go(func() error {
        return process(ctx, item)
    })
}
if err := g.Wait(); err != nil {
    return fmt.Errorf("process batch: %w", err)
}
```

`errgroup.WithContext` cancels the shared context on the first error, and the remaining goroutines wind down on their own. `SetLimit` keeps parallelism
bounded. Before Go 1.22 the loop variable had to be copied inside the loop; that is no longer necessary — but check the version in `go.mod` before
removing the copy from older code.

When more flexible control is needed, use `golang.org/x/sync/semaphore` with weights — for example, so that heavy tasks occupy several slots.

> **In go-atlas.** Code inside this repository uses [`core/runtime/concurrency`](../../core/runtime/concurrency/README.md) instead of a hand-rolled
> `errgroup` + semaphore: `Process` / `ProcessCollect` with `WithConcurrency`, `WithStopOnError`, or an adaptive `WithLimitFunc` give the same bounded,
> cancel-on-error semantics. See `AGENTS.md`, section "Reuse core/*".

### singleflight

A hundred goroutines simultaneously discover that the cache is empty and all go to the database for the same thing. This is the classic thundering herd,
and the fix is a single construct:

```go
var g singleflight.Group

func (s *Service) User(ctx context.Context, id string) (*User, error) {
    v, err, _ := g.Do(id, func() (any, error) {
        return s.repo.LoadUser(ctx, id)
    })
    if err != nil {
        return nil, err
    }
    return v.(*User), nil
}
```

Pitfall: the first caller's context is captured by the closure. If it is canceled, every waiter receives the error, including those still willing to
wait. When that matters, run the shared work under its own bounded context (for example, `context.WithoutCancel` plus a timeout) and use `DoChan` with a
`select` on each caller's `ctx.Done()` only to abandon that caller's wait.

Second pitfall: an error is shared with every member of the group. Only one caller should retry it; otherwise you get the same herd, merely shifted in
time.

### Context and cancellation

Three rules that save a lot of time during incident analysis:

- The context is passed as the first argument and is never stored in a struct.
- Every network call has a deadline. `context.Background()` in a request handler is a bug.
- A child call's deadline is shorter than its parent's. Otherwise the outer timeout fires before the inner one, and you lose the ability to retry
  meaningfully.

### Graceful shutdown

The shutdown order matters, and the reverse of the startup order is not always the right answer:

1. Stop accepting new work (call `Shutdown` on the HTTP server, unsubscribe the consumer).
2. Let in-flight work finish, under a shared deadline.
3. Flush buffers: metrics, logs, ClickHouse batches.
4. Close database and broker clients — only after everything that needs them has finished.

The deadline for all of this must be shorter than `terminationGracePeriodSeconds` in Kubernetes; otherwise SIGKILL will terminate the process in the
middle of step 2.

### Races that are detected automatically

`go test -race` in CI is mandatory. It does not catch everything, but what it catches, it catches reliably. In addition: use `sync.Once` for lazy
initialization, `atomic.Int64` and its siblings instead of a mutex around a counter, and `atomic.Pointer` for lock-free hot swapping of configuration.

---

## 2. Multiple instances, one database

This is where most real-world problems live, and also where they are most often overcomplicated.

### Optimistic locking

Works on the principle "read, modify, verify that nobody got in the way". It is cheap, holds no locks, and fits well when conflicts are rare.

```sql
UPDATE accounts
   SET balance = balance - $1,
       version = version + 1
 WHERE id = $2 AND version = $3;
```

Zero updated rows means someone got there first: re-read, recompute, retry. Cap the number of attempts.

In MongoDB, the same is done with a condition in the filter:

```go
res, err := coll.UpdateOne(ctx,
    bson.M{"_id": id, "version": expectedVersion},
    bson.M{"$inc": bson.M{"version": 1}, "$set": bson.M{"status": "paid"}},
)
// res.MatchedCount == 0 → conflict
```

### Pessimistic locking

`SELECT ... FOR UPDATE` locks the row until the end of the transaction. Use it when conflicts are frequent and re-reading costs more than waiting. Keep
the transaction short: a locked row combined with a slow third-party API call produces a queue of waiters and hung connections.

`FOR UPDATE SKIP LOCKED` still locks the rows it selects but skips rows already locked by others, which turns it into a work-distribution mechanism:
each worker takes whatever is free and does not wait. It is the foundation of a queue in Postgres.

### A unique index as the race arbiter

The most underrated tool on this list. "Check that it does not exist yet, then create it" does not need a lock — it needs a constraint:

```sql
CREATE UNIQUE INDEX ON payments (idempotency_key);
```

Whoever inserts first wins; the others get a conflict and read the winner's result. No locks, no race between check and insert, and it works with any
number of instances. Rule: **if the task is phrased as "check and create", it is a unique index, not a lock.**

### Atomic operations instead of read-modify-write

```go
// bad: race between read and write
doc := load(id); doc.Counter++; save(doc)

// good
coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"counter": 1}})
```

The same applies to SQL: `SET counter = counter + 1`, not a value computed in Go and substituted in. Obvious in theory, routinely violated in code.

### Advisory locks in Postgres

`pg_advisory_lock(key)` provides a named database-level mutex that is not tied to a row. It is convenient for "only one instance runs this data
migration". Session-level locks are released when the connection drops, transaction-level locks on commit; the latter are safer because they cannot
leak.

---

## 3. Distributed level

### Distributed locks, honestly

A distributed lock does not guarantee correctness. At all. The classic scenario: an instance acquires a lock for 30 seconds, then hits a 35-second GC
pause or network delay; the lock expires and is granted to another instance, while the first one wakes up and carries on, convinced it still holds the
lock. Now there are two holders.

Redlock does not solve this scenario, because the problem lies not in the lock service but in the absence of feedback from the protected resource. There
are two solutions:

1. **Fencing token.** The lock issues a monotonically increasing number; the resource remembers the last one it has seen and rejects writes with a lower
   number. This requires support on the resource side, which in practice means a condition such as `UPDATE ... WHERE fence_token < $1`.
2. **Do not rely on the lock for correctness.** Treat it as an optimization that reduces duplicate work, and enforce correctness at the database level —
   with a unique index or an idempotent operation.

We take the second path almost always.

If a lock is genuinely needed, the minimal correct Redis implementation is:

```go
token := uuid.NewString()
ok, err := rdb.SetNX(ctx, key, token, ttl).Result()
```

Release it only with a script that compares the token; otherwise you will release someone else's lock after your own has timed out:

```lua
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
else
  return 0
end
```

A TTL is always mandatory. A lock without a TTL becomes permanent if its holder crashes.

### Leader election

Use it when exactly one active instance is required — a scheduler, an outbox poller, a handler that cannot run in parallel. Options: `etcd` with leases,
or JetStream KV with a TTL on the key, which we already have. In Kubernetes, use `coordination.k8s.io/Lease` via `client-go/tools/leaderelection`.

Important: time passes between losing leadership and realizing it. The leader must verify that it is still the leader before every significant
operation, not just once at startup.

### Periodic jobs in a cluster

Three instances of a service with the same cron inside will run the job three times. The most common symptom is notifications and reports that arrive in
triplicate.

Leader election is overkill here. A uniqueness constraint on the time slot is enough:

```sql
INSERT INTO scheduled_runs (job, slot)
VALUES ('daily_digest', date_trunc('day', now()))
ON CONFLICT DO NOTHING
RETURNING id;
```

If a row is returned, this instance runs the job. If not, someone else has already taken it. No locks, no elections, no extra component, and after a
restart the job does not run again for the same slot. Match the slot granularity to the schedule. Note that this is at-most-once claiming: if the winner
crashes after the insert but before the job completes, the run is lost. When that matters, record execution status in the row and let a later tick
reclaim stale claims, or enqueue the job transactionally.

---

## 4. Consistency between writing and publishing

### Dual write: the root of the problem

Writing to the database and sending to the broker cannot be done atomically. Whatever you do first, there is a moment between the two actions where the
process can die. Send first — the event exists, the data does not. Write first — the data exists, the event is lost. No amount of retrying fixes this,
because it is the process itself that fails.

Almost all of the patterns below follow from this.

### Transactional outbox

The event is written to an `outbox` table **in the same transaction** as the business data. A separate process reads the table and publishes.

```sql
BEGIN;
  UPDATE orders SET status = 'paid' WHERE id = $1;
  INSERT INTO outbox (id, subject, payload) VALUES ($2, 'billing.order.paid', $3);
COMMIT;
```

The poller picks up a batch with `SKIP LOCKED`, publishes it, and marks it as sent. Multiple pollers do not interfere with each other. In MongoDB, a
change stream on the outbox collection reduces delivery latency, but it is an optimization on top of the poller, not a replacement: retries, expired
leases, and missed notifications still need polling (see [Message Brokers and Queues, section 8](message-brokers.md)).

What to understand about the outbox:

- Delivery is at-least-once. A duplicate occurs when publishing succeeded but marking did not. It is eliminated by deduplication on `Nats-Msg-Id` (see
  [Message Brokers and Queues](message-brokers.md)) or by idempotency on the receiving side.
- Publishing order is not guaranteed with parallel pollers. If order matters, there must be a single poller taking records in order — which then limits
  throughput.
- The outbox table must be cleaned up. Delete sent rows or move them to an archive partition; otherwise it becomes the second-largest table in the
  database.

### Inbox and idempotency

The other side. The consumer records the ID of the processed message in a table with a unique index, in the same transaction as the processing result. A
redelivery hits the conflict and is discarded.

Levels of idempotency, from best to worst:

1. The operation is naturally idempotent: `SET status = 'paid'` instead of `attempts = attempts + 1`.
2. A unique index on a business key; the duplicate is caught by an insert conflict.
3. An inbox table with a TTL.
4. An "already done?" check before acting. The race remains, so this is not a solution but the illusion of one.

### Idempotency key in an external API

The same idea, turned outward. The client sends an `Idempotency-Key` header; the server stores the key along with the result and, on a repeat with the
same key, returns the stored response without executing the operation again. This is mandatory for anything that charges money or creates entities: the
network drops at the moment of the response, the client does not know the outcome, and it retries.

Subtlety: the key must be recorded **before** the operation starts, not after; otherwise two concurrent requests with the same key will both go through.
Insert the key with an "in progress" status; an insert conflict means "someone is already handling this".

### Saga

A distributed transaction as a chain of local steps with compensations. Each step commits in its own database; on failure, compensating actions run in
reverse order.

Compensation is not rollback. A canceled charge leaves a trace in the history; a sent email cannot be recalled. Design starting from the question "what
do we do if step five fails", not from the happy path.

Choreography (each participant reacts to events) is simpler at first, but six months later nobody can draw the full flow. Orchestration (a coordinator
drives the process) requires an extra component, but the process is visible in one place. The threshold is roughly at the fourth step.

### Why not 2PC

Two-phase commit provides atomicity at the cost of locking all participants for the duration of the protocol and halting completely if the coordinator
fails. In microservices, this means the failure of one service stalls the others. We use sagas and idempotency instead.

---

## 5. Overload protection

### Timeouts

The first thing to configure and the first thing to be forgotten. Under load, a call without a timeout accumulates goroutines and connections until
something runs out.

The budget is computed top-down: if the incoming request has 3 seconds, two sequential downstream calls get 1.2 seconds each, and the rest goes to your
own work. If the sum of downstream timeouts exceeds the incoming one, your retries will never run — the outer deadline cuts them off first.

### Retries

Three mandatory properties: exponential backoff, jitter, and an attempt limit.

Without backoff, you turn a transient failure in someone else's API into your own DDoS attack. Without jitter, all instances retry in sync and hit in
waves. Without a limit, a single broken message loops forever.

Most importantly: **only idempotent operations may be retried.** A repeated `POST /payments` without an idempotency key is a second charge. Distinguish
transient errors (timeout, 503, connection refused) from permanent ones (400, 422, invalid JSON): retrying the latter is pointless; they go straight to
triage.

### Circuit breaker

When a dependency is down, continuing to hit it is doubly harmful: you waste your own resources and keep it from recovering. After N failures, the
breaker switches to the open state and fails fast for a while, then lets a probe request through.

In Go, use `sony/gobreaker`. The failure threshold, the open-state duration, and the number of probe requests are configurable. An important detail:
each dependency gets its own breaker; a single breaker shared by all outbound calls is useless.

### Bulkhead

Resource isolation, so that one consumer's failure does not exhaust the entire pool. Example: a separate connection pool or a separate semaphore for
reports, so that heavy exports do not take every database connection and bring down the user-facing API. The name comes from shipbuilding: a breached
compartment does not sink the ship.

### Rate limiting

Limiting on your side keeps you from overloading a dependency. Limiting at the entry point keeps clients from overloading you.

Within a process, use `golang.org/x/time/rate` (token bucket). Across instances, use Redis: `INCR` with `EXPIRE` for a fixed window (simple, but allows
a burst at the window boundary) or a sorted set for a sliding window (more accurate, more expensive). Off the shelf: `go-redis/redis_rate`.

### Load shedding and backpressure

When the system is overloaded, an honest rejection beats slow degradation: a `429` with `Retry-After` is more useful than a timeout at the thirtieth
second. Shed load at the entry point, before the request has consumed resources.

Backpressure is the same idea applied upstream: a slow consumer must slow down the producer rather than buffer in memory. Within a process, a bounded
(buffered) channel provides it — the sender blocks when the buffer is full; an unbounded in-memory queue does not. Across a broker, pull consumption
protects the consumer (it takes only what it can process) but does not throttle publishers; that requires admission control on the publishing side or
a stream limit that rejects new messages (`DiscardNew` in JetStream) combined with publisher-side handling of the rejection — a plain size limit may
silently evict old messages instead.

---

## 6. Caching and its races

### Cache stampede

A popular key expires, and a thousand requests simultaneously go to recompute it. Remedies, in order of increasing complexity:

1. `singleflight` within an instance — removes most of the problem almost for free.
2. A distributed lock on recomputation: whoever acquires it computes; the others wait or serve the stale value.
3. Probabilistic early refresh: as the TTL approaches expiry, the chance that a given request refreshes the value ahead of time increases. The herd is
   spread over time and never forms.
4. Different TTLs with jitter for keys of the same kind. If you populated the cache in a single pass with identical TTLs, it will all expire at once.

### Invalidation

Between updating the database and invalidating the cache there is a window in which a stale value can be written to the cache and stay there until the
next TTL expiry. The order "database first, then delete the key" is safer than "cache first", but it does not close the window completely.

Practical takeaway: **every cached value must have a TTL, even if you are confident in your invalidation.** The TTL is insurance against invalidation
failing at some point — and at some point it will fail.

### Negative caching

Cache the absence of a result too, briefly. Otherwise a lookup for a nonexistent key hits the database every time, which is a ready-made vector for
enumeration attacks.

---

## 7. Anti-patterns

1. **A distributed lock instead of a unique index.** More complex, slower, and it still does not provide the guarantee the index does.
2. **A lock without a TTL.** A crashed holder blocks the system until someone intervenes manually.
3. **Releasing someone else's lock.** Deleting the key without comparing the token releases a lock that another holder acquired after your timeout.
4. **Publishing an event without an outbox.** Data and events will diverge; the only question is when.
5. **Retrying a non-idempotent operation.** This is where double charges come from.
6. **Retrying without backoff and jitter.** A transient failure turns into an avalanche.
7. **A call without a timeout.** It accumulates goroutines until something is exhausted.
8. **Read-modify-write instead of an atomic operation.** It loses updates, and the higher the load, the more often.
9. **Cron in every instance.** The job runs as many times as there are pods.
10. **An unbounded `go func()` in a loop.** The service takes down the database, not the other way around.
11. **A long transaction with an external call inside.** The lock is held for the duration of a network operation that may take a minute.
12. **A cache without a TTL, counting on perfect invalidation.** A stale value lives forever.
13. **`context.Background()` in a request handler.** Cancellation never arrives, and work continues after the client has gone.

---

## 8. Defaults

| Task | Solution |
|---|---|
| Bounding goroutine parallelism | `errgroup` with `SetLimit`; in go-atlas, `core/runtime/concurrency.Process` |
| Deduplicating identical requests | `singleflight` |
| "Check and create" | Unique index, not a lock |
| Concurrent entity updates | Optimistic locking on a version field |
| Processing a job queue | `SKIP LOCKED` in Postgres (River) |
| Publishing events | Transactional outbox |
| Protection against reprocessing | Inbox with a unique index |
| State-changing external API | Idempotency key |
| Periodic jobs in a cluster | Uniqueness per time slot |
| Single active instance | Lease in JetStream KV or etcd |
| Retries | Exponential backoff with jitter, attempt limit |
| Protection against a failed dependency | `sony/gobreaker` per dependency |
| Rate limiting across instances | Redis, `redis_rate` |

---

## 9. Review checklist

Questions to ask when reviewing any code that works with shared state:

- [ ] What happens if this handler runs twice concurrently?
- [ ] What happens if the process crashes between these two lines?
- [ ] Is this operation idempotent? If not, what protects it?
- [ ] Does every external call have a timeout? Does it fit within the incoming request's budget?
- [ ] Is parallelism bounded? What happens with a thousand items instead of ten?
- [ ] Is the event published directly from the transaction?
- [ ] Do both the lock and the cache have a TTL?
- [ ] Is a transaction held open during a network call?
- [ ] Does this periodic job run in every instance?
- [ ] Does the package pass `go test -race`?

---

## Appendix: FAQ

**Do we need a distributed lock if we have three instances?** Most likely not. First check whether the problem is solved by a unique index, an atomic
`UPDATE ... WHERE`, or `SKIP LOCKED`. A lock remains appropriate for cases where parallel execution does not produce a wrong result but merely wastes
resources.

**Isn't an outbox for every event overkill?** Overkill is investigating, six months later, why service A has the order marked as paid while service B
knows nothing about it. The cost of an outbox is one extra insert in the same transaction and one background process per service. The cost of not having
one is paid at the worst possible moment.

**Can we assume there will be no duplicates because JetStream deduplicates?** No. The deduplication window is time-bounded, and a retry that arrives
after it will go through. Deduplication reduces the frequency of duplicates; idempotency makes them harmless. You need both, but only the latter can be
relied upon.

**Optimistic or pessimistic locking?** It depends on how often conflicts occur. If they are rare, use optimistic locking — retrying is cheaper than
waiting. If they are frequent, use pessimistic locking; otherwise you will spin on retries. If conflicts are frequent and the operation is long, that
usually indicates the entity is too coarse-grained and should be split.

**Should we adopt Temporal to avoid writing sagas by hand?** Consider it once there are several multi-step processes with compensations and waits on
external events, and they make up a noticeable share of the code. For a single saga it does not pay off: it is one more serious component to operate.
