# mongo

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
```

Package `mongo` translates filter AST nodes into MongoDB `bson.M` query documents. Supports all comparison, logical, membership, and string
operations.

A bare identifier used as a condition becomes a boolean field test — `active` translates to `{active: true}`, `!active` to
`{active: {$ne: true}}` — at the root of an expression and on either side of `$and` / `$or` alike.

## Security

`matches()` passes the user-supplied pattern through to MongoDB's `$regex`. Host-side validation compiles it with Go's RE2 engine (which
cannot backtrack) and enforces a length cap, but MongoDB executes the pattern with its own PCRE-family engine, where a crafted pattern can
backtrack catastrophically — a DB-side ReDoS the length cap bounds but does not eliminate. Expose `matches()` only to trusted callers, or
tighten the cap via `filter.WithMaxRegexLength`. `contains()`, `startsWith()`, and `endsWith()` escape their argument with
`regexp.QuoteMeta` and are not affected.
