# store

Storage clients and their tooling. Capabilities in [audit](../audit), [coord](../coord), [reliability](../reliability) and [aip](../aip) define their storage contracts; the MongoDB backends of
those contracts build on the packages here.

## Packages

| Package                      | Description                                                                          |
|------------------------------|--------------------------------------------------------------------------------------|
| [mongo](./mongo)             | MongoDB client wrapper with CSFLE, migrations, transactions and cursor paging        |
| [meilisearch](./meilisearch) | Context-aware Meilisearch client with idempotent index setup                         |
