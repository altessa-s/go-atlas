# etag

```go
import "github.com/altessa-s/go-atlas/core/encoding/etag"
```

Package `etag` produces and compares entity-tags (ETags, RFC 7232) in both strong (`"value"`) and weak (`W/"value"`) forms. It is transport-neutral: the
same tags drive HTTP conditional requests and the resource `etag` field used by gRPC APIs that follow Google AIP-154, which reuses the RFC 7232 syntax
verbatim as the field value. A `Tag` stores the inner opaque-tag plus a weak flag; `String` renders that wire form and `Parse`/`ParseList` recover tags
(and If-None-Match/If-Match lists) from it.

Strong content tags come from a `Generator` with a pluggable hash (default SHA-256), so callers can trade collision resistance for speed. `FromModTime`
builds a weak validator straight from object metadata, no hashing involved.

This is a generation primitive only. HTTP conditional-request handling (304, header wiring) and the AIP-154 mutation check (`ABORTED` on mismatch,
`etag` field wiring) are left to the transport adapters. `ParseVersion` and `CheckVersion` validate an etag issued by `FromVersion` and report
transport-neutral sentinels (`ErrMissing`, `ErrMalformed`, `ErrMismatch`) that a transport maps to its own status.

## Options

| Option                     | Description                                                                          |
|----------------------------|--------------------------------------------------------------------------------------|
| `WithNewHash(NewHashFunc)` | Hash constructor for strong content tags. Defaults to `DefaultNewHash` (SHA-256).    |
| `WithSingleton()`          | `VersionOption`: `ParseVersion`/`CheckVersion` accept `W/"0"`, a singleton never saved. |

`ParseVersion` accepts exactly what `FromVersion` renders: weak, canonical decimal (no sign, leading zeros or surrounding whitespace), and not
`MaxInt64` (the next version would overflow). `W/"0"` needs `WithSingleton`: every other stored version is positive. A store doing compare-and-swap
uses `ParseVersion` and reports its own conflict.

## Key types

| Type          | Description                                                                                          |
|---------------|-----------------------------------------------------------------------------------------------------|
| `Tag`         | An entity-tag (inner value + weak flag). Comparable; the zero value is the absent sentinel.          |
| `Generator`   | Produces strong content tags with a configurable hash. Build with `NewGenerator`; concurrency-safe. |
| `NewHashFunc` | `func() hash.Hash` — returns a fresh hash per call.                                                  |

## Functions

| Function                          | Description                                                                   |
|-----------------------------------|-------------------------------------------------------------------------------|
| `Strong(value)`                   | Strong tag wrapping a precomputed value (`"value"`).                           |
| `Weak(value)`                     | Weak tag wrapping a precomputed value (`W/"value"`).                           |
| `FromModTime(size, mod)`          | Weak validator `W/"<size16>-<unixnano16>"` from object metadata (no hashing).  |
| `FromVersion(v)`                  | Weak etag `W/"<v>"` of a stored integer version (canonical decimal); zero tag for `v < 0`. |
| `Tag.Version()`                   | Version of a `FromVersion` tag; `ErrNotVersion` for a strong, non-canonical, signed or overflowing value. |
| `ParseVersion(s, ...)`            | Version of an etag a client sends back: exactly a `FromVersion` rendering; `ErrMissing` when empty, else `ErrMalformed`. |
| `CheckVersion(s, cur, ...)`       | `ParseVersion`, then `ErrMismatch` (wraps `core/errors.ErrConflict`) when the version is not `cur`. |
| `Random()`                        | Fresh weak opaque token (`crypto/rand`) per call — cheap version/OCC tag, **not** a cache/range validator. |
| `Parse(s)`                        | Parse one entity-tag; errors wrap `ErrInvalidTag`.                             |
| `ParseList(s)`                    | Parse an If-None-Match/If-Match list; `*` yields `star=true`.                  |
| `Generator.Hash([]byte)`          | Strong tag of an in-memory body.                                              |
| `Generator.HashString(string)`    | Strong tag of a string (zero-copy).                                           |
| `Generator.HashReader(io.Reader)` | Strong tag of a stream, without buffering it whole.                           |
| `Tag.String()`                    | Wire form (`"v"` or `W/"v"`); empty for the zero tag.                         |
| `Tag.Value()` / `IsWeak()` / `IsZero()` | Inner opaque-tag, weak flag, absent-tag check.                          |

## Comparison

`Tag` comparison follows RFC 7232 §2.3.2. The zero (absent) tag never matches.

| Method               | Semantics                              | Used by                          |
|----------------------|----------------------------------------|----------------------------------|
| `Tag.StrongMatch(o)` | Both tags strong **and** values equal. | If-Match, ranges, AIP-154 mutate |
| `Tag.WeakMatch(o)`   | Values equal, weakness ignored.        | If-None-Match                    |

## Usage

### Generate and serve an ETag

```go
g := etag.NewGenerator()
tag := g.HashString(body) // `"<sha256-hex>"`
w.Header().Set("ETag", tag.String())
```

### Honor If-None-Match

```go
inm, star, err := etag.ParseList(r.Header.Get("If-None-Match"))
if err == nil && (star || slices.ContainsFunc(inm, tag.WeakMatch)) {
    w.WriteHeader(http.StatusNotModified)
    return
}
```

### gRPC / AIP-154 resource validation

The same `Tag` drives an AIP-154 `etag` resource field: store `String()` into it, and validate a mutation against the request `etag` with a strong
comparison, returning `ABORTED` on mismatch.

```go
cur := g.HashString(resource) // current resource tag
if req.GetEtag() != "" {       // empty request etag skips validation
    want, err := etag.Parse(req.GetEtag())
    if err != nil || !want.StrongMatch(cur) {
        return nil, status.Error(codes.Aborted, "etag mismatch")
    }
}
resp.Etag = cur.String() // `"<sha256-hex>"`, ready for the proto field
```

### Cheap weak validator from file metadata

```go
fi, _ := os.Stat(path)
tag := etag.FromModTime(fi.Size(), fi.ModTime()) // W/"<size16>-<unixnano16>" (both base-16)
```

## See also

- [`hash`](../hash) — the SHA-256 hex helpers reused for strong digests.
