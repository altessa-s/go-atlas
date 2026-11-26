# mongo

```go
import "github.com/altessa-s/go-atlas/store/mongo"
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

| Option                            | Default           | Description                                                            |
|-----------------------------------|-------------------|------------------------------------------------------------------------|
| `WithKMS`                         | nil               | KMS provider for CSFLE                                                 |
| `WithVaultDatabase`               | same as main      | Database for encryption keys                                           |
| `WithVaultCollection`             | `__keyVault`      | Collection for encryption keys                                         |
| `WithEncryptionEnabled`           | false             | Enable Client-Side Field Encryption                                    |
| `WithEncryptionModel`             | --                | Per-struct field encryption config                                     |
| `WithBSONTagName`                 | `bson`            | Tag name for BSON field mapping                                        |
| `WithEncryptionTagName`           | `encryption`      | Tag name for encryption mapping                                        |
| `WithClient`                      | nil               | Pre-initialized `mongo.Client`                                         |
| `WithClientOptions`               | nil               | Custom driver `*options.ClientOptions` (`mongo/options`)               |
| `WithTransactionOptions`          | snapshot/majority | Transaction read/write concerns                                        |
| `WithLogger`                      | discard           | Structured logger                                                      |
| `WithMeter`                       | no-op             | Metrics collector for MongoDB operations                               |
| `WithConverterOptions`            | --                | Extra `converter.Option`s threaded into `GetEntity`/`GetEntities`      |
| `WithReadDeduplication`          | off               | Share eligible in-flight repository reads |
| `WithDeduplicationIdentity`       | nil               | Per-context identity segment for opted-in read deduplication |
| `WithRollbackMigrationsOnFailure` | off               | Roll back this run's applied migrations when one fails (see Migrations) |

## Write documents

**BREAKING CHANGE.** `Mongo.ConvertToNewDocument` and `Mongo.ConvertToUpdateDocument` now use the same BSON layout as the
[MongoDB behavior translators](../../mapping/behavior/translators/mongo/README.md). Both accept a struct or a non-nil pointer chain to one.

| Input | Insert | Update |
|-------|--------|--------|
| Anonymous struct field | Nested under its BSON name by default | Same nesting |
| Explicit `inline` | Promotes struct fields or string-map entries. A nil inline pointer contributes no keys. | Same promotion. An empty inline map contributes no operator. |
| Field names | A bare tag names the BSON key under the default tag key. Only the complete `bson:"-"` skips a field. `bson:"-,omitempty"` names the key `-`. | Same names. A dotted name or a leading `$` in an addressed path is an error. |
| Duplicate names | The shallowest inline candidate wins. Equal-depth duplicates, invalid inline declarations and colliding inline-map entries fail. | Same layout checks |
| `omitempty` | Uses driver emptiness, including `IsZero`. Scalar zero is omitted. A non-nil pointer to an ordinary zero scalar is present. | Scalar zeros are set. Nil pointers and empty collections become `$unset`. |
| Collections without `omitempty` | Nil maps and slices become BSON null. Empty collections retain their BSON type, including empty binary for `[]byte{}`. | Replaces nonempty collections. Embedded element identifiers are written with the elements. |
| Codecs and `minsize` | Uses BSON types from the effective registry, including across pointers and collections. `bson.D` stays an ordered document, and small `int64` values tagged `minsize` can become BSON int32. | Honors codecs, addressability of pointer-receiver marshalers and inherited `minsize`. |
| Empty struct or all fields excluded | Empty document | Empty `$set` |
| `_id`, `behavior:"identifier"`, `omitonupdate` | Written under the normal insert rules | Excluded from record updates. Neither operator targets the root `_id` or its descendants. |

Standard inserts use the driver's root encoder, including a root `MarshalBSON`, when no field needs encryption. The result contains BSON values such as
`bson.DateTime`, `bson.D`, `bson.A` and `bson.Binary`. The driver can stringify supported non-string map keys on insert. The standard insert
checks the encoded document recursively for keys beginning with `$`. A custom BSON tag key selects field-by-field construction, including nested
struct values. Folded maps require string keys and reject keys beginning with `$`.

Updates replace ordinary subdocuments in full. For example, a nil child pointer disappears from its parent's replacement. If replacing a parent would
remove a protected field, the builder uses dotted paths for the writable siblings. Unsetting the entire protected parent fails with
`ErrProtectedUnset` from the translator. Store uses an empty behavior strip set, so `immutable` and `output_only` alone allow writes. The behavior
engine validates kind names and enforces its depth limit on both operations.

Encryption declarations use Go field names in `WithEncryptionModel` or field tags such as `encryption:"deterministic,user_email"`. The model takes
precedence. Register rules for promoted fields on the embedded type that declares them. The builder encrypts selected values before returning the
document, including fields inside a replacement subdocument or collection. If a codec hides an encrypted descendant or encryption fails, the error
matches `ErrFieldEncryption`. Document encoding and CSFLE plaintext encoding use the same client registry. Each Atlas-created client gets a private
registry unless its client options specify one. A wrapper around an external client with no recorded registry uses private Atlas defaults.

`New` validates model rules at construction. Unknown, unexported and promoted field names, duplicate rules, and missing keys or algorithms are
errors. A selected field containing another selected field also fails with `ErrFieldEncryption`. The constructor checks registered model types, and
the insert and update builders check types discovered at conversion. Use `deterministic` or `random` on the field to encrypt. To encrypt a
descendant, declare the rule on that descendant. The former `nested` algorithm marker is unnecessary and fails algorithm validation when applied.

## Read filters

**Breaking change.** Entity readers, offset lists and cursor lists now use the same strict BSON filter contract as keyset pagination.
Pass `bson.D`, `bson.M`, `map[string]any`, `bson.Raw`, or nil. See the [filter migration notes](keyset/README.md#migrating-keyset-filters)
for struct replacements, ordered embedded documents and scalar codec registration.

Each read prepares the filter with the Atlas client's effective registry and BSON options. For an external client the defaults are the
Atlas registry and driver BSON defaults. Supply database or collection overrides explicitly. Entity readers accept a trailing
`FilterEncoding` (the last settings value wins). For lists use `WithListFilterEncoding` or `WithListCursorFilterEncoding`.

```go
encoding := mongo.FilterEncoding{Registry: registry, BSONOptions: bsonOptions}
user, err := mongo.GetEntity[UserDocument, User](ctx, client, collection, filter, encoding)
```

A new `ListCursor` continuation binds the prepared BSON bytes, including numeric types and field order. Older signed tokens and stored
records still use their original JSON filter hash and the same collection, subject and sort checks. Older releases cannot resume new
BSON-bound cursors. `ListCursorSeq` captures the prepared filter when iteration begins and reuses it across pages.

## Read deduplication and tenant isolation

`GetEntity` and `GetEntities` issue independent queries by default. Pass `mongo.WithReadDeduplication()` to `mongo.New` to let concurrent reads
share an in-flight query when the database, collection handle, model type, operation, prepared BSON filter, and caller identity match. Reuse the
same collection handle across requests. Each caller decodes and converts its own BSON copy. Provide database or collection codec overrides through
`FilterEncoding`. On a borrowed handle, pass its registry and any nondefault BSON options to enable sharing. With no explicit settings, borrowed
handles use the driver's decoder and issue independent reads.

When caller identity affects a result outside the filter, identical filters can return different data. Per-tenant CSFLE keys, session-scoped reads,
and caller-bound read preferences can make it unsafe to share a query across tenants. Reads carrying a driver session always run independently to
preserve its read scope and snapshot. Distinct collection handles also use distinct queries.

Canceling one waiter does not cancel shared work or other waiters. The shared query keeps the first caller's context values and is bounded by
`DefaultQueryTimeout`. A deduplicated read may join a query that started before the caller's own acknowledged write. Call either helper with
`mongo.WithFreshRead(ctx)` or pass `mongo.FilterEncoding{FreshRead: true}` after a write. The collection's read preference and read concern still apply.

In such deployments, configure `WithDeduplicationIdentity` so the identity (tenant/subject) is mixed into the deduplication key. Callers
share a query only when their identities match.

```go
m, _ := mongo.New("app", mongo.WithReadDeduplication(), mongo.WithDeduplicationIdentity(func(ctx context.Context) string {
    p, _ := principal.FromContext(ctx)
    return p.Tenant // "" leaves the identity segment empty
}))
```

Returning `""` or leaving the option unset uses an empty identity segment.

`GetEntities` returns a slice and therefore retains the result set. For large or unbounded reads, `GetEntitiesSeq` decodes and converts one document at
a time and yields the first query, decode, or conversion error through `iter.Seq2`; it does not deduplicate concurrent iterations:

```go
for user, err := range mongo.GetEntitiesSeq[UserDocument, User](ctx, client, collection, filter) {
    if err != nil {
        return err
    }
    consume(user)
}
```

## Cursor modes

`ListCursor` issues its "next" token in one of three forms, chosen by configuration and never by the shape of the token a client presents:

| Mode               | Option                  | Token                         | Client can alter              | Subject and collection binding |
|--------------------|-------------------------|-------------------------------|-------------------------------|--------------------------------|
| Stateful           | `WithListCursorStorage` | ULID key of a stored record   | Nothing                       | Enforced                       |
| Stateless signed   | `WithListCursorSigner`  | Signed (or encrypted) payload | Nothing                       | Enforced                       |
| Stateless unsigned | neither (default)       | Base64 JSON                   | Position within its own query | None; a subject is refused     |

`WithListCursorSubject` needs one of the first two modes. An unsigned token carries no subject, and one it carried could be rewritten by the client, so
`ListCursor` given a subject without a storage or a signer fails with `ErrCursorSubjectNotEnforced` — on the first page too — instead of serving the
request unbound. With both a storage and a signer the cursor is stored and the signer is unused.

## Stateful cursor isolation

Cursor storage snapshots the supplied metadata and gives each reader an independent copy. Backends that support explicit removal also implement
`CursorDeleter`.

In stateful mode (`WithListCursorStorage`), `ListCursor` returns an opaque ULID token and keeps the pagination metadata server-side. The token is a
bearer credential: any caller presenting it — on the same collection, with the same filter and sort — continues that pagination. If a token leaks or is guessed, another principal can
page through the original principal's result set.

**Contract:** in multi-tenant deployments, bind the cursor to the principal with `WithListCursorSubject`. The subject is recorded when the "next" token
is minted and re-checked on every continuation; a mismatch returns `ErrCursorSubjectMismatch`:

```go
p, _ := principal.FromContext(ctx)
res, err := mongo.ListCursor[User](ctx, coll,
    mongo.WithListCursorStorage(store),
    mongo.WithListCursorSubject(p.Tenant), // "" leaves the cursor unbound
)
```

The match is exact. A cursor minted without a subject — by an unscoped listing over the same collection, or before the endpoint started passing a
subject — continues only under a request that passes none; a request with a subject rejects it with `ErrCursorSubjectMismatch`, as a request without a
subject rejects a bound cursor. Binding stays opt-in: callers that never pass a subject are unaffected.

The cursor format follows the configuration, not the token's shape: with storage configured, `ListCursor` accepts only a ULID storage key and rejects a
stateless base64 token or a direct `*Cursor` (`WithListCursorCursor(*Cursor)`) with `ErrInvalidCursorFormat`, so a client cannot mint its own cursor and
skip the stored metadata and subject check.

A stored cursor is bound to the query it was issued for. The record carries a digest of the database and collection names, so a token presented to a
`ListCursor` call on another collection fails with `ErrCursorCollectionMismatch`, and endpoints that share one storage do not accept each other's
tokens. The continuation must also repeat the sort and cursor-ID field of the first page, as in stateless mode: a different sort — or an omitted one,
which requests the default sort — fails with `ErrCursorChecksumMismatch` instead of being replaced by the cursor's. Together with the filter hash
(`ErrCursorFilterMismatch`, whose text never spells the hashes) and the subject, this leaves two endpoints interchangeable only when they page the same
collection with the same filter, sort and subject; use a storage (or key prefix) per endpoint, or a subject that includes the endpoint, where even that
matters.

## Signed stateless cursors

`WithListCursorSigner` signs stateless tokens with a go-aip
[`pagination/pagetoken`](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken) codec, the mechanism `keyset.FindPage` uses:
HMAC-SHA256, previous keys for rotation, a token TTL (24h by default) and optional AES-256-GCM encryption. Nothing is stored server-side:

```go
codec, err := pagetoken.New(signingKey) // or factory/ext/aip/pagination/pagetoken from the paging config
res, err := mongo.ListCursor[User](ctx, coll,
    mongo.WithListCursorSigner(codec),
    mongo.WithListCursorSubject(p.Tenant),
)
```

The signed payload holds the cursor (position, sort, cursor-ID field, filter hash), the digest of the database and collection names, and a digest of
the subject. A continuation is checked like a stored one, and with the same errors: the collection (`ErrCursorCollectionMismatch`), the filter
(`ErrCursorFilterMismatch`), the subject, exactly, an unbound token included (`ErrCursorSubjectMismatch`), and the sort and cursor-ID field
(`ErrCursorChecksumMismatch`). A token that does not verify — tampered, signed with an unknown key, expired, issued by another list sharing the codec,
or an unsigned base64 token — fails with `ErrInvalidCursor`; the signer's own error stays in the chain, so `pagetoken.Classify` still tells an expired
token from a malformed one. A direct `*Cursor` fails with `ErrInvalidCursorFormat`. Unsigned tokens issued before the signer was configured are not
accepted for any window: their holders restart pagination.

The payload is bounded by the codec (`pagetoken.MaxPayloadLength`, 1024 bytes): a page ending on a long string sort value fails to issue its token;
raise the bound with `pagetoken.WithMaxPayloadLength` or use stateful mode. Signed tokens are readable unless the codec encrypts
(`pagetoken.WithEncryption`). `ListCursor` is configured per call, so there is no config block for the signer: pass the codec the service already builds
for its other page tokens.

## Stateless cursor integrity

An unsigned stateless cursor is client-held, and its checksum is a plain SHA-256 of the sort and cursor-ID field with no secret. `ListCursor`
therefore treats every field as untrusted: the embedded sort and cursor-ID field must equal the request's (else `ErrCursorChecksumMismatch`), the filter hash must match
(`ErrCursorFilterMismatch`), and the sort value must decode to a BSON scalar — string, boolean, number, Decimal128, date, timestamp, ObjectID, binary or
null — else `ErrInvalidCursor`. The tie-breaker equality is rendered as `{field: {$eq: value}}`. A client can still edit the cursor ID and sort value to
start a page at another position within the same filter; use stateful mode or `WithListCursorSigner` where that is not acceptable.
The cursor is base64 JSON, not encrypted: it shows the client the cursor ID and sort value of the last item of the page. Use stateful mode, or a signer
that encrypts (`pagetoken.WithEncryption`), when the sort field is hidden from the client by a projection.
`ParseCursor` rejects a token longer than `MaxCursorTokenLength` (16 KiB) before decoding it, and `ErrCursorChecksumMismatch` never spells the sort or
cursor-ID field the server expects.
The continuation uses the last item's raw BSON sort value. A missing sort key and BSON null share a segment, ordered by the configured cursor ID.
Keep the populated sort values within one BSON comparison type. An unsupported boundary such as an array, embedded document, or non-ObjectID
cursor ID fails with `ErrMissingCursorKey`. A non-ID sort requires a continuation with an explicit boundary and rejects tokens that lack one
with `ErrInvalidCursor`.

## Cursor sort shapes

A cursor resumes on the value of one sort field, with the cursor-ID field as the tiebreaker in the same direction. `ListCursor` therefore accepts a
single sort field, that field followed by the cursor-ID field in the same direction, or a sort led by the cursor-ID field. Any other shape — a second
non-ID key, or an ID tiebreaker in the opposite direction — would skip or repeat documents and fails with `ErrUnsupportedCursorSort`.

## Keyset pages

Import `github.com/altessa-s/go-atlas/store/mongo/keyset`.

`keyset.FindPage[T](ctx, coll, codec, keyset.Query{...})` reads one page with a plain `find`, positioned by a signed
go-aip [`pagination/pagetoken`](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken) token instead of
an offset. It works with any `_id` type (ObjectID, string, UUID), needs no `created_at` field and streams documents, so a page is not bound by the 16 MB
document limit that the `ListCursor` aggregation has.

| Field         | Meaning                                                                                                                    |
|---------------|----------------------------------------------------------------------------------------------------------------------------|
| `Filter`      | Query predicate, marshaled once: the token is bound to those bytes and the same bytes run (see below)                      |
| `Sort`        | `bson.D` of fields with `1` / `-1` (go-aip `backend/mongo` order translator); `_id` ascending is the tie-breaker           |
| `Projection`  | Returned fields; keep the sort fields; touching one with anything but plain inclusion/exclusion fails with `ErrProjection` |
| `Limit`       | Page size, `DefaultPageSize` (100) when not positive, at most `MaxPageSize`                                                |
| `After`       | Token of the page to read; empty for the first page                                                                        |
| `Subject`     | Principal the token is bound to                                                                                            |
| `Registry`    | Filter registry, inherited from an Atlas-created client or defaulting to `bsoncodec.NewRegistry()` for an external client |
| `BSONOptions` | Filter encoder options, inherited from an Atlas-created client or defaulting to driver defaults for an external client     |

A token replayed under another filter, sort or subject fails with the `pagetoken` errors. Every document needs a comparable scalar of one BSON type in
each
sort field. When issuing `Next`, a missing, null or array sort value in the last document fails with `ErrMissingKey`. A projection that drops
or rewrites a sort field fails with `ErrProjection` before the find. A sort direction other than `1`/`-1`, or an empty or repeated field, fails
with `ErrSort`. A nil codec reads a single page with no `Next` token, and a non-empty `After` then fails with `keyset.ErrTokensRequired`
(it wraps `pagetoken.ErrCodecRequired`, so `pagetoken.Classify` reports a malformed token). Index the sort fields (plus `_id`) so each page is a range
scan. Paging is keyset-based, not a snapshot.

The token binds the validated BSON bytes that `find` executes. Within the filter, maps encode in sorted key order.
Use `bson.D` or pre-built `bson.Raw` for embedded documents whose field order matters. For a custom scalar struct codec,
call `bsonfilter.AdmitScalar` on the registry that encodes the filter. Pass `Registry` and `BSONOptions` explicitly for
external clients with custom settings and for database or collection overrides. See the
[upgrade notes](keyset/README.md) for existing tokens and struct filters.

```go
page, err := keyset.FindPage[Order](ctx, coll, codec, keyset.Query{
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

## Transactions and upserts

| Helper                                       | Behavior                                                                                                       |
|----------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| `(*Mongo).WithTransaction(ctx, fn)`          | Runs `fn` in a transaction; joins a running transaction of the same client carried by `ctx` instead of nesting |
| `InTransaction(ctx)`                         | `ctx` carries a session with a running transaction (a session without one does not count)                      |
| `Transaction(ctx)`                           | `uow.Detector`: the running transaction, by session and driver transaction number (a restart differs)          |
| `SetOnInsertFrom(doc, filterKeys...)`        | `$setOnInsert` document of an upsert: `doc`'s top-level fields minus the filter keys, encoded once             |
| `UpsertRetryingDuplicate(ctx, attempts, fn)` | Repeats `fn` at once on a duplicate key (first-insert race), up to `attempts`; exactly once in a transaction   |
| `Now()`, `StoredTime(t)`                     | Time as MongoDB stores it: UTC, floored to the millisecond, no monotonic reading                               |

A joined `WithTransaction` runs its handler on the enclosing session: its writes commit or abort with the enclosing transaction, its error goes to the
enclosing handler, and it applies no timeout, metric or log of its own. A transaction of another client is never joined.

`SetOnInsertFrom` encodes `doc` with the default Atlas codecs (decimal, dates, `Optional`) and encoder settings, so struct tags apply as
for `InsertOne`; the fields keep `doc`'s order and are `bson.RawValue`s that any registry writes unchanged. A filter key the document omits is ignored.

`UpsertRetryingDuplicate` retries only `mongo.IsDuplicateKeyError`; another error, or the last duplicate, is returned as is. Inside a transaction a
failed write aborts the transaction, so the upsert runs once and only a retry of the whole transaction can recover.

## Write guards

`TouchGuard(ctx, coll, filter, field, upsert, opts...)` increments the storage-only counter `field` of the document `filter` matches and reports whether
a document was touched. The step is an int32 1 (the encoding of `$inc: 1`), or an int64 1 with `WithInt64Increment()` for counters kept as int64; an
existing int64 counter stays int64 either way. Inside a transaction it serializes conflicting writers through that document: a concurrent transaction
that writes the same document — another `TouchGuard`, a delete, any update — gets a transient write conflict, and `WithTransaction` retries it.

| Use                   | Call                                                                                               | A miss means                                  |
|-----------------------|----------------------------------------------------------------------------------------------------|-----------------------------------------------|
| Reference check       | `TouchGuard(ctx, targets, bson.M{"_id": ref, "deleted": bson.M{"$ne": true}}, "ref_guard", false)` | unknown or deleted target (`false`, no error) |
| Per-key lock document | `TouchGuard(ctx, guards, bson.M{"_id": key}, "n", true)`                                           | never: the guard is inserted                  |

Without upsert a miss writes nothing. With upsert two first writers of one guard racing outside a transaction can fail with a duplicate key, returned
wrapped (`IsErrorDuplicate`). Outside a transaction `TouchGuard` is a plain write and serializes nothing beyond it. The counter value has no meaning.

## Migrations

Register steps through `github.com/altessa-s/go-atlas/store/mongo/migrations`. For an independent set, add steps to a `migrations.Registry` with
`Register` and supply it to `New` with `WithMigrationRegistry`. That client runs only its registry.

To populate the default registry, register migrations from an `init` function with an explicit version, without importing the third-party registry:

```go
func init() {
    migrations.MustRegisterMigration(5, "orders_backfill_status", backfillStatus, nil) // nil down: irreversible, rollback stops here
}
```

| Function                                                | Behavior                                                                                                 |
|---------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| `migrations.RegisterMigration(version, desc, up, down, opts...)` | Registers a migration. Version 0, a nil `up`, or a duplicate version returns `migrations.ErrInvalidMigration`. |
| `migrations.MustRegisterMigration(...)`                 | Same, panics on an invalid migration                                                                     |
| `migrations.RegisteredMigrations()`                     | The registered migrations by version (for tests asserting the set)                                       |

Versions are stored in the same `migrations` collection and format (`{version, description, timestamp}`) as `mongo-migrate`, whose global registry
(versions taken from file names) is still honored: a migration applied earlier under the same version never runs again. A version registered in both
registries fails `Connect` with `migrations.ErrInvalidMigration`.

`Close` waits for an in-flight `Connect` to finish, including ping retries and schema migration. The shutdown timeout starts after that wait.

`migrations.WithCleanupOnFailure` registers a per-step handler for partial changes if that step's up returns an error. It runs under the
verified lease before any rollback of earlier steps. The failed step stays pending and its down is never called. See the
[cleanup contract](migrations/README.md#cleanup-after-a-failed-step).

`Connect` applies all pending migrations before returning. When one fails, `Connect` returns its error and the migrations applied before
it stay applied; `WithRollbackMigrationsOnFailure` instead rolls back the steps this run applied, newest first, while the lease is held; earlier runs' steps are never touched and a step registered with a nil `down` is irreversible (`migrations.ErrIrreversibleMigration`). The underlying
library reads the current schema version and writes the new one with nothing in between, so two replicas starting together would both read version N and
both run migration N+1. Migration bodies are arbitrary code rather than idempotent DDL, so running one twice is not generally safe.

The package therefore takes a database-wide lock around the migration run. It is a single document in a **separate** collection — `migrations_lock`,
never `migrations`, because `mongo-migrate` derives the schema version from the greatest `_id` in its own collection and would decode a lock document
there as version 0.

| Constant                      | Default           | Description                                              |
|-------------------------------|-------------------|----------------------------------------------------------|
| Lock collection               | `migrations_lock` | Collection holding the mutex document                    |
| `migrations.LockWaitTimeout`  | `30s`             | How long a replica waits for a peer to finish migrating  |
| `migrations.MigrationTimeout` | `30s`             | Budget for the migrations themselves, and the lock lease |

Waiters block rather than skip: once the holder finishes, the next waiter acquires the lock, re-reads the version, and finds nothing left to apply. The
lease is compared against the server clock (`$$NOW`), so replicas with drifted clocks cannot steal a live lock. The lease equals
`migrations.MigrationTimeout`; a heartbeat renews it every third of a lease until the work returns, counting validity from the send time of the last
renewal minus a margin. The same value caps the work context. When the budget ends, a step that ignores its context keeps the lease and is still
recorded in the ledger when it returns; when the lease is lost (a renewal finds it taken, or none succeeded in time) the work is canceled, no
rollback runs and no ledger record is written.

This is a TTL lock, not a fencing one: it bounds the damage from a crashed holder, but the `mongo-migrate` API offers no place to carry a fencing token.
For deployments that want concurrency gone as a class, run migrations from a dedicated deploy step (init container, Job) and keep the lock as the
backstop.

## Indexes

Import `github.com/altessa-s/go-atlas/store/mongo/indexes`.

`indexes.Ensure(ctx, coll, models, opts...)` creates the indexes of a collection idempotently at every start. Without options it is one
`createIndexes` command: an equal existing index is a no-op and a conflicting one fails it with the server's error.

| Option                               | Behavior                                                                                               |
|--------------------------------------|--------------------------------------------------------------------------------------------------------|
| `indexes.WithDropObsolete(names...)` | Drops the named indexes when present, before creating the models (missing index or collection is fine) |
| `indexes.WithDropOnSpecMismatch()`   | On `IndexOptionsConflict`/`IndexKeySpecsConflict`, reconciles under a per-collection lock (below)      |

Reconciliation runs while holding the mutex document `indexes:<collection>` in `migrations_lock` (lease `indexes.IndexLockLease`, wait
`indexes.IndexLockWaitTimeout`),
so replicas starting together take turns. Under the lock every model is created afresh; an index is dropped and recreated only when that fresh create
conflicts, an index of the model's name exists, and its full current specification is not provably equal to the model — so a peer's freshly built index
is never dropped on a stale conflict. The server decides what a mismatch is, so an equal index is never rebuilt; the same keys under another name are
never dropped (name it in `indexes.WithDropObsolete` instead). Dropping a unique index leaves a window without the constraint until it is rebuilt, and
the
rebuild fails if the data violates the new specification. Unnamed models get the driver's generated name (`status_1_created_at_-1`); duplicate names, an
obsolete name that is also a model's, `_id_` or an empty name are `indexes.ErrInvalidIndexModels`. The outbox, saga and audit MongoDB storages create
their
indexes with it.

## Subpackages

| Package                                                  | Description                                                       |
|----------------------------------------------------------|-------------------------------------------------------------------|
| [factory](../../factory/store/mongo)                     | Configuration-based creation                                      |
| [bsoncodec](./bsoncodec)                                 | BSON registry for Optional, decimal and calendar dates            |
| [cas](./cas)                                             | Optimistic-concurrency filters, version bump, miss classification |
| [kms](./kms)                                             | KMS provider interface                                            |
| [kms/providers/local](./kms/providers/local)             | Local key management                                              |
| [kms/providers/aws](./kms/providers/aws)                 | AWS KMS provider                                                  |
| [kms/providers/azure](./kms/providers/azure)             | Azure Key Vault provider                                          |
| [kms/providers/gcp](./kms/providers/gcp)                 | Google Cloud KMS provider                                         |
| [factory/store/mongo/kms](../../factory/store/mongo/kms) | KMS factory from config                                           |
| [storages/memory](./storages/memory)                     | In-memory cursor storage                                          |
| [storages/redis](./storages/redis)                       | Redis-backed cursor storage                                       |
| [storages/nats](./storages/nats)                         | NATS-backed cursor storage                                        |
| [mongotest](./mongotest)                                 | Env-gated connection for integration tests                        |

## Package structure

| Package                            | Responsibility                                |
|------------------------------------|-----------------------------------------------|
| [keyset](keyset/README.md)         | MongoDB keyset pagination                     |
| [indexes](indexes/README.md)       | Index creation and reconciliation             |
| [migrations](migrations/README.md) | Migration registry and database runner        |
| [explain](explain/README.md)       | Aggregation explain execution and diagnostics |
| [bsoncodec](bsoncodec/README.md)   | BSON codecs                                   |
| [kms](kms/README.md)               | Encryption key provider contracts             |
| [cas](cas/README.md)               | Optimistic concurrency helpers                |
| [storedoc](storedoc/README.md)     | Stored document error classification          |
| [mongotest](mongotest/README.md)   | MongoDB test fixtures                         |

Internal packages keep implementation details out of the public API:

| Package                 | Responsibility                                                                         |
|-------------------------|----------------------------------------------------------------------------------------|
| `internal/document`     | CSFLE field selection and encryption for the shared BSON builder |
| `internal/query`        | Legacy sort syntax and read defaults. Filters use go-mongox `bsonfilter.Prepare`.      |
| `internal/pipeline`     | Offset/cursor aggregation stages and independent total counts                          |
| `internal/databaselock` | Database leases shared by migrations and index reconciliation                          |

The root retains connection lifecycle, transactions, key management and the legacy pagination API. These share client state or public cursor defaults.
Extracted APIs are available only from their owning subpackages; the root has no compatibility wrappers. The BSON translators own document layout.

The legacy sort syntax (`name,-age`) is distinct from AIP `order_by`. Both pagination APIs use the shared parser.
