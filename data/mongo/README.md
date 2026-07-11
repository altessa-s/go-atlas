# mongo

```go
import "github.com/altessa-s/go-atlas/data/mongo"
```

Package `mongo` provides a MongoDB client wrapper with Client-Side Field Level Encryption (CSFLE), transaction handling, connection pooling, and
structured logging.

## Requirements

- **MongoDB 4.4+** — required by the cursor-paginated `ListCursor` aggregation. The count branch uses `$unionWith` (introduced in 4.4) to keep
  `total` stable across pages independent of the cursor position. Offset-based `List` only needs the `$facet` baseline (3.4+), but the rest of
  the package is built and tested against 4.4+.
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
| `WithClientOptions`      | nil               | Custom `mongo.ClientOptions`                                      |
| `WithTransactionOptions` | snapshot/majority | Transaction read/write concerns                                   |
| `WithLogger`             | discard           | Structured logger                                                 |
| `WithConverterOptions`   | --                | Extra `converter.Option`s threaded into `GetEntity`/`GetEntities` |
| `WithDeduplicationIdentity` | nil            | Per-context identity segment for the read singleflight key (see below) |

## Read deduplication and tenant isolation

`GetEntity` and `GetEntities` collapse concurrent identical reads through a `singleflight` group keyed by `db:collection:hash(filter)`. Two callers
issuing the **same** filter at the same time run one query and share its result. That is correct only when the result is a pure function of the
filter. If caller identity affects what a query returns but lives **outside** the filter — e.g. per-tenant CSFLE data keys, a read scope applied by a
session, or a read preference bound to the caller — then collapsing identical filters across tenants can hand one tenant another's result.

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

**Contract:** in multi-tenant deployments, bind the cursor to the principal with `WithListCursorSubject`. The subject is recorded when the "next"
token is minted and re-checked on every continuation; a mismatch returns `ErrCursorSubjectMismatch`:

```go
res, err := mongo.ListCursor[User](ctx, coll,
    mongo.WithListCursorStorage(store),
    mongo.WithListCursorSubject(principal.FromContext(ctx).Tenant()), // "" leaves the cursor unbound
)
```

Binding is opt-in: a cursor minted without a subject stays replayable by any caller, so existing callers are unaffected.

## Subpackages

| Package                                              | Description                   |
|------------------------------------------------------|-------------------------------|
| [factory](./factory)                                 | Configuration-based creation  |
| [kms](./kms)                                         | KMS provider interface        |
| [kms/local](./kms/local)                             | Local key management          |
| [kms/aws](./kms/aws)                                 | AWS KMS provider              |
| [kms/azure](./kms/azure)                             | Azure Key Vault provider      |
| [kms/gcp](./kms/gcp)                                 | Google Cloud KMS provider     |
| [kms/factory](./kms/factory)                         | KMS factory from config       |
| [cursor_storages/memory](./cursor_storages/memory)   | In-memory cursor storage      |
| [cursor_storages/redis](./cursor_storages/redis)     | Redis-backed cursor storage   |
| [cursor_storages/nats](./cursor_storages/nats)       | NATS-backed cursor storage    |
| [cursor_storages/kvstore](./cursor_storages/kvstore) | KV store-based cursor storage |
