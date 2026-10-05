# NATS KeyValue bucket storage migration

The NATS JetStream server cannot change the storage type of an existing KeyValue bucket. Every NATS KV backend in go-atlas therefore adopts an
existing bucket whose storage type differs from the configured one as is, logging a warning on every start, or fails with `ErrBucketStorageMismatch`
when `WithStrictBucketStorage()` is set. To actually move such a bucket, each backend exports `MigrateBucketStorage`, which recreates the bucket on
the configured storage while every process using it is stopped. This page covers what the backends share; each backend README covers what happens
to its own entries.

| Backend          | Package                                                                    | First argument | Target storage                    | Entries                                              |
|------------------|----------------------------------------------------------------------------|----------------|-----------------------------------|------------------------------------------------------|
| Idempotency      | [`data/idempotency/storages/nats`](../../data/idempotency/storages/nats)   | `js`           | file                              | copied, with their remaining lifetimes               |
| Rate limiter     | [`data/limiters/storages/nats`](../../data/limiters/storages/nats)         | `js`           | file                              | copied                                               |
| Saga             | [`data/saga/storages/nats`](../../data/saga/storages/nats)                 | `js`           | file                              | copied                                               |
| Distributed lock | [`data/locks/dlock/providers/nats`](../../data/locks/dlock/providers/nats) | `nc`           | `WithStorage` (memory by default) | not copied: no lock is held while all are stopped    |
| Leader election  | [`data/leadelect/providers/nats`](../../data/leadelect/providers/nats)     | `nc`           | `WithStorage` (memory by default) | not copied: there is no leader while all are stopped |

## Procedure

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production, for example:

   ```go
   err := nats.MigrateBucketStorage(ctx, js, nats.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `nats.MigrationOptions{Resume: true}`. Until the migration
   completes, `New` fails with `ErrBucketMigrationInProgress` (a `KVMIGRATE_<bucket>` stream marks it), and a run without `Resume` refuses to
   continue it. A migrator holds a lease on the bucket (in the `kvmigrate_leases` bucket, renewed every 10s, expiring after 30s): a second
   migration, fresh or resumed, fails with `ErrBucketMigrationLocked` while one runs or until a crashed one's lease expires, and a migrator
   that lost its lease stops. The lease does not fence stream operations on the server, so still confirm a crashed run has exited before
   resuming. The bucket names `kvmigrate_leases` and `kvmigrate_*` are reserved.
4. Check that `nats kv info <bucket>` reports the target storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

A missing bucket, or one already on the target storage, is left alone.

## What the migration does

The migration seals the bucket, copies its live entries into the marker stream (for the backends that copy entries), recreates the bucket with its
first revision just above the old bucket's last one, and restores the entries. Revisions therefore keep growing across the migration, and so do the
fencing tokens derived from them.

It rejects a JetStream context with a domain or API prefix (`ErrMigrationUnsupportedContext`) and a bucket with sources, republishing, a subject
transform or a placement (`ErrMigrationUnsupportedBucket`). Recreating a sourced bucket would either replay old source values over newer local writes
or lose source messages not yet copied, since the server does not expose how far each source was consumed. Republishing would announce every restored
entry as a new change, and the others cannot be validated without a cluster.

A bucket key TTL other than the configured one fails with `ErrBucketTTLMismatch` unless `WithMigrateBucketTTL()` is passed too, which changes both at
once.

## Mirrors

A **mirror** bucket is migrated by re-sync. It is recreated as the same mirror on the new storage, nothing is copied, and the new mirror catches up
from its origin. It keeps the origin's revisions, so fencing stays monotonic. It holds what the origin still holds: entries the old mirror kept but
the origin has aged out are gone. Check the mirror's lag (`nats stream info KV_<bucket>`) before starting its readers.

## Entry lifetimes and clock accuracy

For the backends that copy entries, an entry already past its deadline is not restored. Remaining lifetimes are computed from the server's message
timestamps and the migrator's clock, so run the migration on a host whose clock is synchronized with the NATS servers. A migrator clock ahead of the
servers drops entries that still had that much lifetime left, and one behind keeps entries that much longer. Transport delay between reading the clock
and the server storing an entry adds the same kind of error, in the order of a round trip.
