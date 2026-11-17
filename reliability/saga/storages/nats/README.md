# nats

```go
import natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
```

Durable [`saga.Storage`](../../storage.go) backed by a NATS JetStream KeyValue bucket. Each instance is a JSON document keyed by its ID, and the
bucket's revision is used directly as the optimistic-concurrency token (`saga.Instance.Version`) — so two coordinators cannot advance the same instance,
and state survives process restarts for crash recovery.

## Options

| Option            | Default | Description                                                                                  |
|-------------------|---------|----------------------------------------------------------------------------------------------|
| `WithBucket`      | `saga`  | KeyValue bucket name.                                                                         |
| `WithBucketTTL`   | 30 days | Backstop time-to-live for instances. Active sagas reset it on every checkpoint; terminal ones stay until the TTL expires or the caller invokes `Delete`. Tune to your longest saga lifetime plus retention. |
| `WithMigrateBucketTTL` | off | Update an existing bucket whose key TTL differs from `WithBucketTTL`. Without it `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched. YAML (saga factory): `storage.nats.migrateBucketTTL`. |

## Behavior

| Method             | Notes                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------|
| `Create`           | `KV.Create`; `errs.ErrInstanceExists` on a duplicate ID. Writes the new revision into `Version`. |
| `Get`              | `errs.ErrInstanceNotFound` when absent; `Version` reflects the current revision.               |
| `Update`           | Revision-checked `KV.Update`; `errs.ErrVersionConflict` on a stale `Version`, `errs.ErrInstanceNotFound` when gone. |
| `FetchRecoverable` | Scans every key (NATS KV has no queries) and returns non-terminal instances mid-compensation, past their deadline, or with an expired execution lease. |
| `Delete`           | Idempotent.                                                                                     |

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so instance IDs are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). ULIDs and UUIDs are stored
as they are; `order:42` is stored as `order=3A42`. A key that is already valid and contains no `=` is stored under itself. The escaped key may be at
most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O. An ID that is not valid UTF-8 fails with
`ErrInvalidID`: the instance is stored as JSON, which would rewrite the ID.

## Usage

```go
nc, _ := nats.Connect(nats.DefaultURL)
js, _ := jetstream.New(nc)
store, err := natsstore.New(ctx, js, natsstore.WithBucket("saga"))

orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
inst, err := orch.Start(ctx, id, data)
```

## Bucket storage

New buckets are file-backed. The server cannot change a bucket's storage type, so `New` returns `ErrBucketStorageMismatch` and leaves the bucket
untouched when it already exists with another one; align the configuration, recreate the bucket, or use another bucket.
