# factory

```go
import natsfactory "github.com/altessa-s/go-atlas/factory/store/mongo/storages/nats"
```

Maps the `nats` block of the storage backend configuration to a [NATS JetStream KeyValue cursor storage](../../../../../store/mongo/storages/nats/) for
[`factory/store/mongo`](../..), which builds only the in-memory storage.

## Usage

```go
storage, err := mongofactory.New(cfg.CursorStorage).
    UseStorage(storageconfig.BackendTypeNATS, natsfactory.Builder(js)).
    WithTTL(24 * time.Hour).
    Build(ctx)
```

| Symbol                     | Description                                                                              |
|----------------------------|------------------------------------------------------------------------------------------|
| `Builder(js)`              | Storage function for the `nats` type; keeps cursors in a JetStream KeyValue bucket       |
| `DefaultBucket`            | Bucket name used when `nats.bucket` is empty (`cursor`)                                  |
| `ErrJetStreamRequired`     | Returned when the function has no JetStream context                                      |
| `ErrConfigRequired`        | Returned when the configuration has no `nats` block                                      |
| `ErrBucketTTLMismatch`     | Returned when the bucket exists with another or no key TTL and `migrateBucketTTL` is off |
| `ErrBucketStorageMismatch` | Returned when the bucket exists with another storage type                                |

## Bucket

The bucket (`nats.bucket`, default `cursor`) is created as file storage with the `WithTTL` key TTL of the cursor storage builder. The TTL is
mandatory: cursors are never deleted, so the bucket's key TTL is their only cleanup, and a build without `WithTTL` or with a zero or negative TTL
fails with `coreerrs.ErrInvalidArgument` before the bucket is touched. `natsstorage.DefaultTTL` (1h) is the recommended value. An existing bucket is
checked like in every other NATS KV backend:

- a different key TTL — a bucket created without a TTL included — fails with `ErrBucketTTLMismatch` and leaves the bucket untouched, unless
  `nats.migrateBucketTTL: true`, which updates the bucket's TTL in place; entries older than the new TTL then expire.
- a different storage type fails with `ErrBucketStorageMismatch` and leaves the bucket untouched: the server cannot convert it.
