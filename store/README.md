# store

The packages here provide backend clients and application transaction infrastructure. Storage contracts belong to
[audit](../audit), [coord](../coord), [reliability](../reliability) and [go-aip](https://github.com/altessa-s/go-aip).

## Packages

| Package                      | Description                                                                          |
|------------------------------|--------------------------------------------------------------------------------------|
| [mongo](./mongo)             | MongoDB client wrapper with CSFLE, migrations, transactions and cursor paging        |
| [meilisearch](./meilisearch) | Context-aware Meilisearch client with idempotent index setup                         |
| [s3probe](./s3probe)         | Signed HeadBucket access check for S3-compatible storage through the guarded HTTP client |
| [postgres](./postgres)       | PostgreSQL pools, transactions and observability over database/sql       |
