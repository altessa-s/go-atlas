# collections

Generic, pure-function collection utilities for Go. Each subpackage focuses on a single collection type and can be imported independently.

## Subpackages

| Package                  | Description                                                        |
|--------------------------|--------------------------------------------------------------------|
| [maps](./maps)           | Map transformation, filtering, flat/nested conversion, pooling, weak references |
| [slices](./slices)       | Slice transformation, filtering, deduplication, grouping, pooling  |

## Design principles

- **Pure functions** — inputs are never modified; new collections are returned.
- **Nil-in, nil-out** — empty or nil inputs return nil, following Go convention.
- **Generics-first** — all functions use type parameters; no `reflect` or `any` casts internally.
- **Iterator support** — lazy `iter.Seq` / `iter.Seq2` iterators (Go 1.23+) alongside materialized variants.
- **Pooling** — concurrency-safe `sync.Pool` wrappers to reduce GC pressure in hot paths.
