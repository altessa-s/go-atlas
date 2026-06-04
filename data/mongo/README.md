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
