# mongo

```go
import "github.com/altessa-s/go-atlas/data/mongo"
```

Package `mongo` provides a MongoDB client wrapper with Client-Side Field Level Encryption (CSFLE), transaction handling, connection pooling, and
structured logging.

## Requirements

- **MongoDB 5.0+** — `Connect` rejects older servers with `ErrUnsupportedVersion` (`MinRequiredMongoMajorVersion`). The cursor-paginated `ListCursor`
  aggregation's count branch uses `$unionWith` (introduced in 4.4) to keep `total` stable across pages independent of the cursor position; offset-based
  `List` only needs the `$facet` baseline (3.4+).
- CSFLE features additionally require a libmongocrypt-compatible driver and a configured KMS provider.

## Features

- Client-Side Field Level Encryption with multi-cloud KMS support
- Transaction handling with configurable read/write concerns
- Connection pooling and health checks
- Struct-level encryption configuration via tags or programmatic API

## Options

| Option                   | Default           | Description                                                       |
|--------------------------|-------------------|-------------------------------------------------------------------|
| `WithKMS`                | nil               | KMS provider for CSFLE                                            |
| `WithVaultDatabase`      | same as main      | Database for encryption keys                                      |
| `WithVaultCollection`    | `__keyVault`      | Collection for encryption keys                                    |
| `WithEncryptionEnabled`  | false             | Enable Client-Side Field Encryption                               |
| `WithEncryptionModel`    | --                | Per-struct field encryption config                                |
| `WithBSONTagName`        | `bson`            | Tag name for BSON field mapping                                   |
| `WithEncryptionTagName`  | `encryption`      | Tag name for encryption mapping                                   |
| `WithClient`             | nil               | Pre-initialized `mongo.Client`                                    |
| `WithClientOptions`      | nil               | Custom driver `*options.ClientOptions` (`mongo/options`)          |
| `WithTransactionOptions` | snapshot/majority | Transaction read/write concerns                                   |
| `WithLogger`             | discard           | Structured logger                                                 |
| `WithCollector`          | no-op             | Metrics collector for MongoDB operations                          |
| `WithConverterOptions`   | --                | Extra `converter.Option`s threaded into `GetEntity`/`GetEntities` |
| `WithDeduplicationIdentity` | nil            | Per-context identity segment for the read singleflight key (see below) |
| `WithRollbackMigrationsOnFailure` | off      | Roll back every applied migration when one fails (see Migrations) |

## Parser metadata

`Parser` caches type descriptions and binds field values for each `ParseStruct` call. Nil embedded pointers omit promoted fields only for that instance;
a later non-nil instance of the same type is parsed fully. The cache retains no entity values.

Returned metadata belongs to the caller. `NestedStructs` contains type descriptions, without instance values. Encryption models are copied at
construction. The unused `WithParserUpdate` and `WithParserEncryption` options were removed; operation modes belong to document processing.

## Read deduplication and tenant isolation

`GetEntity` and `GetEntities` collapse concurrent identical reads through a `singleflight` group keyed by `db:collection:hash(filter)`. Two callers
issuing the **same** filter at the same time run one query and share its result. That is correct only when the result is a pure function of the filter.
If caller identity affects what a query returns but lives **outside** the filter — e.g. per-tenant CSFLE data keys, a read scope applied by a session,
or a read preference bound to the caller — then collapsing identical filters across tenants can hand one tenant another's result.

A read whose context carries a session (inside `WithTransaction` or any `mongo.NewSessionContext`) is never deduplicated: it must see its transaction's
snapshot, and a read outside the transaction must never see its uncommitted writes.

**Contract:** in such deployments, configure `WithDeduplicationIdentity` so the identity (tenant/subject) is mixed into the deduplication key. Callers
are then only collapsed when they share an identity:

```go
m, _ := mongo.New("app", mongo.WithDeduplicationIdentity(func(ctx context.Context) string {
    return principal.FromContext(ctx).Tenant() // "" disables prefixing (backward-compatible default)
}))
```

Returning `""` (or leaving the option unset) keeps the key identical to the un-prefixed form, so single-tenant callers are unaffected.

## Stateful cursor isolation

In stateful mode (`WithListCursorStorage`), `ListCursor` returns an opaque ULID token and keeps the pagination metadata server-side. The token is a
bearer credential: any caller presenting it — with the same filter — continues that pagination. If a token leaks or is guessed, another principal can
page through the original principal's result set.

**Contract:** in multi-tenant deployments, bind the cursor to the principal with `WithListCursorSubject`. The subject is recorded when the "next" token
is minted and re-checked on every continuation; a mismatch returns `ErrCursorSubjectMismatch`:

```go
res, err := mongo.ListCursor[User](ctx, coll,
    mongo.WithListCursorStorage(store),
    mongo.WithListCursorSubject(principal.FromContext(ctx).Tenant()), // "" leaves the cursor unbound
)
```

Binding is opt-in: a cursor minted without a subject stays replayable by any caller, so existing callers are unaffected.

The cursor format follows the configuration, not the token's shape: with storage configured, `ListCursor` accepts only a ULID storage key and rejects a
stateless base64 token or a direct `*Cursor` (`WithListCursorCursor(*Cursor)`) with `ErrInvalidCursorFormat`, so a client cannot mint its own cursor and
skip the stored metadata and subject check. Storage origin does not bind a token to an endpoint: endpoints that share one storage accept each other's
tokens when filter and subject match. Use a storage (or key prefix) per endpoint, or a subject that includes the endpoint, where that matters.

## Stateless cursor integrity

A stateless cursor is client-held, and its checksum is a plain SHA-256 of the sort and cursor-ID field with no secret. `ListCursor` therefore treats
every field as untrusted: the embedded sort and cursor-ID field must equal the request's (else `ErrCursorChecksumMismatch`), the filter hash must match
(`ErrCursorFilterMismatch`), and the sort value must decode to a BSON scalar — string, boolean, number, Decimal128, date, timestamp, ObjectID, binary or
null — else `ErrInvalidCursor`. The tie-breaker equality is rendered as `{field: {$eq: value}}`. A client can still edit the cursor ID and sort value to
start a page at another position within the same filter; use stateful mode or a signed [`data/keyset`](../keyset) token where that is not acceptable.
Resuming a sort on an embedded-document or array field is not supported: issuing such a cursor fails with `ErrInvalidCursor`.

For reliable continuation, the primary sort field must be present, non-null and have a consistent BSON type across the result set. The legacy cursor
format omits a null/missing boundary and falls back to the cursor ID, which can skip items when another field defines the sort order. Scalar validation
prevents query injection; it does not make null or mixed-type sort boundaries safe for pagination.

## Cursor sort shapes

A cursor resumes on the value of one sort field, with the cursor-ID field as the tiebreaker in the same direction. `ListCursor` therefore accepts a
single sort field, that field followed by the cursor-ID field in the same direction, or a sort led by the cursor-ID field. Any other shape — a second
non-ID key, or an ID tiebreaker in the opposite direction — would skip or repeat documents and fails with `ErrUnsupportedCursorSort`.

## Keyset pages

`FindPage[T](ctx, coll, codec, KeysetQuery{...})` reads one page with a plain `find`, positioned by a signed [`data/keyset`](../keyset) token instead of
an offset. It works with any `_id` type (ObjectID, string, UUID), needs no `created_at` field and streams documents, so a page is not bound by the 16 MB
document limit that the `ListCursor` aggregation has.

| Field        | Meaning                                                                                                   |
|--------------|-----------------------------------------------------------------------------------------------------------|
| `Filter`     | Query predicate, bound into the token                                                                     |
| `Sort`       | `bson.D` of fields with `1` / `-1` (`ParseSortString`, data/orderby); `_id` ascending is the tie-breaker  |
| `Projection` | Returned fields; keep the sort fields; touching one with anything but plain inclusion/exclusion fails with `ErrKeysetProjection` |
| `Limit`      | Page size, `DefaultKeysetPageSize` (100) when not positive, at most `MaxKeysetPageSize`                   |
| `After`      | Token of the page to read; empty for the first page                                                       |
| `Subject`    | Principal the token is bound to                                                                           |

A token replayed under another filter, sort or subject fails with the `keyset` errors. Every document needs a comparable scalar of one BSON type in each
sort field. Only the last document of a page is checked, when its `Next` token is issued: a missing, null or array value there (an excluded sort field
included) fails with `ErrKeysetMissingKey` rather than skipping documents. A sort direction other than `1`/`-1`, or an empty or repeated field, fails
with `ErrKeysetSort`. A nil codec reads a single page with no `Next` token, and a non-empty `After` then fails with `ErrKeysetTokensRequired`. Index the
sort fields (plus `_id`) so each page is a range scan. Paging is keyset-based, not a snapshot.

```go
page, err := mongo.FindPage[Order](ctx, coll, codec, mongo.KeysetQuery{
    Filter:  bson.D{{Key: "tenant", Value: tenant}},
    Sort:    bson.D{{Key: "total", Value: -1}},
    Limit:   50,
    After:   req.PageToken,
    Subject: userID,
})
```

## Errors

`IsErrorDuplicate`, `IsErrorCollectionNotFound`, `IsErrorCollectionExists` (code 48, e.g. a concurrent `CreateCollection` in a migration) and
`IsErrorIndexNotFound` classify server errors through any wrapping.

## Migrations

`Connect` applies all pending `mongo-migrate` migrations before returning. When one fails, `Connect` returns its error and the migrations applied before
it stay applied; `WithRollbackMigrationsOnFailure` instead rolls back every migration — the whole schema, idempotency indexes included. The underlying
library reads the current schema version and writes the new one with nothing in between, so two replicas starting together would both read version N and
both run migration N+1. Migration bodies are arbitrary code rather than idempotent DDL, so running one twice is not generally safe.

The package therefore takes a database-wide lock around the migration run. It is a single document in a **separate** collection — `migrations_lock`,
never `migrations`, because `mongo-migrate` derives the schema version from the greatest `_id` in its own collection and would decode a lock document
there as version 0.

| Constant                    | Default            | Description                                              |
|-----------------------------|--------------------|----------------------------------------------------------|
| `MigrationLockCollectionName` | `migrations_lock`  | Collection holding the mutex document                    |
| `MigrationLockWaitTimeout`  | `30s`              | How long a replica waits for a peer to finish migrating  |
| `MigrationTimeout`          | `30s`              | Budget for the migrations themselves, and the lock lease |

Waiters block rather than skip: once the holder finishes, the next waiter acquires the lock, re-reads the version, and finds nothing left to apply. The
lease is compared against the server clock (`$$NOW`), so replicas with drifted clocks cannot steal a live lock. The lease equals `MigrationTimeout`,
which also caps the holder's work context once the lock is taken; cancellation is cooperative, so a migration that ignores its context can outlive the
lease.

This is a TTL lock, not a fencing one: it bounds the damage from a crashed holder, but the `mongo-migrate` API offers no place to carry a fencing token.
For deployments that want concurrency gone as a class, run migrations from a dedicated deploy step (init container, Job) and keep the lock as the
backstop.

## Subpackages

| Package                                              | Description                   |
|------------------------------------------------------|-------------------------------|
| [factory](./factory)                                 | Configuration-based creation  |
| [bsoncodec](./bsoncodec)                             | BSON registry for Optional, decimal and calendar dates |
| [kms](./kms)                                         | KMS provider interface        |
| [kms/local](./kms/local)                             | Local key management          |
| [kms/aws](./kms/aws)                                 | AWS KMS provider              |
| [kms/azure](./kms/azure)                             | Azure Key Vault provider      |
| [kms/gcp](./kms/gcp)                                 | Google Cloud KMS provider     |
| [kms/factory](./kms/factory)                         | KMS factory from config       |
| [cursorstorages/memory](./cursorstorages/memory)   | In-memory cursor storage      |
| [cursorstorages/redis](./cursorstorages/redis)     | Redis-backed cursor storage   |
| [cursorstorages/nats](./cursorstorages/nats)       | NATS-backed cursor storage    |
| [cursorstorages/kvstore](./cursorstorages/kvstore) | KV store-based cursor storage |
