# cas

```go
import "github.com/altessa-s/go-atlas/data/mongo/cas"
```

Optimistic-concurrency (compare-and-set) helpers for MongoDB documents with an int64 version: an exact-match CAS filter, an atomic
version bump, an anti-ABA initial version, and the NotFound-vs-Conflict classification of a miss.

## API

| Symbol                                                     | Behavior                                                                                                     |
|------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| `InitialVersion()`                                         | Version of a new document: `time.Now().UnixNano()`, above every version of an earlier document with that ID |
| `Filter(idField, id, versionField, expected)`              | CAS selector `{idField: id, versionField: expected}` (exact match)                                           |
| `Inc(versionField)`                                        | Update document `{$inc: {versionField: int64(1)}}`; add the data operators (`$set`, …) to it                 |
| `ClassifyMiss(ctx, coll, idField, id, notFound, conflict)` | After a CAS write matched nothing: `notFound` if the document is absent, `conflict` if it exists             |
| `ErrNotFound`, `ErrConflict`                               | What `ClassifyMiss` returns when the caller passes a nil `notFound` / `conflict`                             |

## Semantics

- Every stored document carries its version; `Filter` matches it exactly, with no fallback for a missing field.
- `Inc` is a plain `$inc` update document rather than a pipeline stage: it composes with the update operators of the same write, so
  the data change and the bump are atomic. A version stored as int64 stays int64.
- `InitialVersion` closes the ABA window of physical deletes: a document deleted and inserted again under the same ID starts at the
  insert time in nanoseconds, so a stale etag of the previous incarnation (its insert time plus its number of writes) cannot match,
  as long as the wall clock does not step back between the two inserts.
- `ClassifyMiss` counts the ID with the caller's context (inside a transaction it reads the transaction's snapshot). It never returns
  nil for a miss: a nil sentinel becomes `ErrNotFound` / `ErrConflict`, which wrap the canonical `core/errors` `ErrNotFound` /
  `ErrConflict`. A failed count is returned wrapped, not classified.

## Usage

```go
update := cas.Inc("version")
update["$set"] = fields
res, err := coll.UpdateOne(ctx, cas.Filter("_id", id, "version", expected), update)
if err != nil {
    return err
}
if res.MatchedCount == 0 {
    return cas.ClassifyMiss(ctx, coll, "_id", id, ErrNotFound, ErrVersionConflict)
}
```

The saga MongoDB store ([`data/saga/storages/mongo`](../../saga/storages/mongo)) writes its version-checked updates with `Filter`,
`Inc` and `ClassifyMiss`.
