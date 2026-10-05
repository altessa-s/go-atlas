# redisfilter

```go
import "github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
```

Shared plumbing for Redis-backed probabilistic filter storages built on RedisBloom module commands. The Bloom (`BF.*`) and Cuckoo (`CF.*`)
storages differ almost only in the command prefix, the reserve arguments, and the filter label in wrapped errors — `Core` captures the common
execution, error-wrapping, batching, and create-on-first-use behavior, parameterized by a `Commands` set.

## Key types

| Type       | Description                                                                                       |
|------------|---------------------------------------------------------------------------------------------------|
| `Commands` | RedisBloom command verbs of one filter type: `Label`, `Exists`, `Add`, `AddTokens`, `AddBatch`, `BatchTokens`, `Reserve`, `Info` (inserts must not create a missing filter, e.g. `BF.INSERT … NOCREATE ITEMS`); `FalseRejects` treats a RESP3 `false` item reply as rejected (`CF.INSERT` on a full filter) |
| `Core`     | Executes the commands against one filter key; safe for concurrent use                              |

## Core methods

| Method         | Description                                                                                                                                            |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| `MightExist`   | Membership check via the exists command                                                                                                                |
| `Add`          | Single insert; creates the filter on a "not exist"/"not found" error and retries once                                                                  |
| `AddBatch`     | Chunked multi-insert (batches of ≤1000 command arguments); same create-and-retry behavior                                                              |
| `EnsureFilter` | Reserve with the configured arguments; an already-existing filter is not an error. Creating a missing filter deletes the ready marker in the same script |
| `Reserve`      | Reserve with explicit arguments; any failure is an error. Clears the ready marker like `EnsureFilter`                                                   |
| `RebuildCommitted` | Whether the ready marker and the filter key exist: some process committed a rebuild and the key was not recreated since                          |
| (first write)  | `Add`/`AddBatch` reserve the filter with the configured arguments once before the first write, so RedisBloom never creates it implicitly with defaults |
| `Stage`        | Reserve a replacement filter under a unique staging key in the live key's hash slot                                                                    |
| `DeleteFilter` | `DEL` of the filter key                                                                                                                                |
| `Info`         | Raw `*.INFO` reply; reports `found=false` (not an error) when the filter does not exist yet                                                            |
| `FilterKey`    | The fully prefixed Redis key                                                                                                                           |

## Staging

`Staging` (from `Core.Stage`) is the replacement filter of an atomic rebuild. `Stage` reserves it and sets a TTL (`StagingTTL`) in one Lua script, so a
staging key never exists without an expiry and a crashed rebuild cannot leak it. `AddBatch` uses the non-creating batch command
(`Commands.StagingAddBatch`, e.g. `BF.INSERT … NOCREATE ITEMS`) and refreshes the TTL after every batch, so a staging key that vanished fails the
rebuild instead of being recreated empty. `Commit` runs one Lua script that renames the staging key onto the live key, strips the TTL (`PERSIST`) and
leaves a short-lived commit marker; if the reply is lost, the marker tells whether the commit happened, and an outcome that cannot be established is
reported as `probfilter.ErrCommitIndeterminate`. `Abort` deletes the staging key. The commit also advances the live filter's generation counter (on
every attempt); `Core.Delete` runs the delete command through a script that checks the generation it observed, so a delete delayed past a rebuild does
nothing. As its last step, after the rename, the commit sets the filter's ready marker; every path of this package that creates the filter key
deletes the marker in the same script, so `RebuildCommitted` is true only while the live key holds a committed rebuild's contents.

All metadata — staging keys, commit markers, generation tokens, ready markers, delete request records — lives in the reserved namespace
`__probfilter__:{<tag>}:<kind>:<hex(live key)>[:<id>]` (`MetaPrefix`). The tag is the live key's hash tag, the live key itself, or a numeric
tag with the same CRC16 slot, so scripts can touch metadata and the live key in Redis Cluster; the hex-encoded live key keeps metadata of
different filters apart. A filter key inside the reserved namespace is rejected (`ErrReservedKey`), so metadata can never overwrite a
filter. `Core.Delete` stores a pending record of each delete request in a per-filter hash without TTL before the delete and its outcome
afterwards, so a client retry of the same request (after a lost reply) never deletes a second, colliding fingerprint; every request also
carries a one-minute execution deadline in server time, and records are pruned only after it. A retry that finds the request still pending,
or arrives after its deadline, fails with `ErrDeleteIndeterminate`. Redis must not evict these keys: use `noeviction` or a `volatile-*`
policy.

Replies are decoded protocol-agnostically: `ToBool` accepts RESP2 integers and RESP3 booleans, and `InfoFields` iterates RESP2 arrays and
RESP3 maps, so the storages work with go-redis v9's default RESP3 as well as RESP2.

Each staging id is created at most once: the stage script records it in a TTL-less registry (hash plus deadline sorted set, pruned only
after the one-minute server-time deadline of the request), so a delayed replay of a stage request cannot recreate a lost staging key. A
commit that finds neither the staging key nor its marker is reported as `ErrCommitIndeterminate`, since a lost reply plus a lost marker
looks the same.

## Rebuild lease

`Core.BeginRebuild` takes a ticket (`INCR`) and the filter's rebuild lease (`SET NX PX`, `LeaseTTL`) in one script before the rebuild reads
its source, and renews it every `LeaseTTL/3` until `Lease.Release`. A held lease makes it fail with `probfilter.ErrRebuildInProgress`.
Staging created through `Lease.Stage` commits only while the lease still holds its ticket and no newer ticket was published; the commit
records its ticket before any promotion step. Otherwise `Commit` returns `probfilter.ErrRebuildSuperseded` without renaming anything.

## Helpers

| Function     | Description                                                         |
|--------------|---------------------------------------------------------------------|
| `InfoFields` | Iterator over the alternating key/value pairs of a `*.INFO` reply   |
| `ToInt64`    | Converts an info reply value (`int64`, `int`, or string) to `int64` |
| `ToBool`     | Converts a boolean reply (RESP2 `0`/`1` or RESP3 boolean) to `bool` |

Batch replies are checked per item: an error element or a negative integer (`CF.INSERT`'s `-1` for a full filter) fails the batch with
`ErrItemRejected`.

## Error wrapping

All failures are wrapped with `core/errors.WrapOperation` using operation strings derived from `Commands.Label`, e.g. for `Label: "Bloom"`:
`check existence in Redis Bloom filter`, `add to Redis Bloom filter`, `batch add to Redis Bloom filter`, `create Redis Bloom filter`,
`delete Redis Bloom filter`, `get Redis Bloom filter info`. These strings are a compatibility contract of the public storages — keep them
byte-identical.

## Usage

See the consumers: [`bloom/storages/redis`](../../bloom/storages/redis/README.md) and [`cuckoo/storages/redis`](../../cuckoo/storages/redis/README.md).
