# Plan B — dotted-path partial updates for nested structures (`data/mongo`)

Status: **deferred design**, not implemented. The shipped behavior is Plan A
(whole-object replacement) — see `data/mongo/mongo_enc.go`
(`ConvertToUpdateDocument`, `processStructPointerField`, `processMapField`).

## Context

`ConvertToUpdateDocument` builds a `{$set, $unset}` update from a Go struct. For
nested structures (pointer-to-struct fields and struct values inside maps) it
currently uses **whole-object replacement**: the nested struct is emitted as a
single `$set` subdocument that fully replaces the stored one, e.g.

```
$set: { parent: { a: 1 } }
```

Consequence: any field already stored under `parent` that is not present in the
Go value is wiped, and nested nil/zero fields are removed implicitly by the
replacement (no nested `$unset` is emitted).

Plan B replaces this with **dotted-path partial updates**:

```
$set:   { "parent.a": 1 }
$unset: { "parent.b": "" }
```

This preserves untouched nested fields already in the database and makes nested
`$unset` first-class and correct.

## Motivation

- Callers doing partial updates usually expect only-changed-fields semantics;
  whole-object replacement of a nested struct is a silent data-loss footgun for
  fields not represented in the Go struct.
- Makes nested field removal explicit and MongoDB-correct.

## Design

Thread a `prefix string` through the recursion (`convertToDocument` →
field processors), and change nested writers to emit dotted keys:

1. `convertToDocument(ctx, entity, update, encrypt, prefix)` — new `prefix` param
   (empty at the top level).
2. **Scalar/default fields** (`processDefaultField`) — key becomes
   `prefix + fieldName` in `$set`; the pre-processing filter's `$unset` entries
   (`applyPreProcessingFilters`) likewise become `prefix + fieldName`.
3. **Struct pointer** (`processStructPointerField`) — instead of
   `setDoc[field] = chSetDoc`, recurse with `prefix + field + "."` and let the
   child write dotted keys directly into the shared `setDoc`/`unsetDoc`. The
   nested `$unset` is now propagated (correctly, as `parent.child`).
4. **Map of structs** (`processMapField`) — recurse per entry with
   `prefix + field + "." + mapKey + "."`. Keep the existing `$`-prefixed map-key
   rejection. Note: dotted keys with special characters in map keys need review
   (MongoDB allows dots in stored keys but not in update paths pre-5.0; validate
   or reject keys containing `.`).

## Hard cases that MUST be handled before shipping

1. **Arrays/slices of structs** (`processSliceField`) — dot-paths cannot address
   array elements without positional operators (`$[]`, `$[<id>]`, numeric index).
   Decision needed: keep whole-array `$set` for slices (mixed model), or support
   positional updates (large scope). Recommended: **keep whole-array replacement
   for slices** in Plan B v1 and document the asymmetry.
2. **CSFLE / encryption** — encrypted values are encrypted as whole values; a
   nested encrypted field must still map to a single dotted `$set` path with the
   encrypted binary. Verify `NestedEncryptionKey` propagation with prefixes and
   that no plaintext leaks via partial paths.
3. **`omitempty` / `omitonupdate`** — semantics must be identical per nested
   field under the prefixed path.
4. **Empty nested object** — if a nested struct yields no `$set` and only
   `$unset`, ensure we do not emit `$set: {parent: {}}` (current quirk) nor an
   empty-path key; emit only the dotted `$unset` entries.
5. **Depth / cycles** — bound recursion depth; guard against pathological nesting.

## Testing

- Table-driven tests mirroring `mongo_enc_nested_test.go` but asserting dotted
  paths: `$set["parent.a"] == 1`, `$unset["parent.b"] == ""`, untouched
  `parent.c` NOT present in the update.
- Map-of-structs dotted paths and map-key validation (reject/escape `.`).
- Slice-of-structs still whole-array replacement (documented).
- CSFLE: encrypted nested field → single dotted `$set` path with binary; error
  when encryption unconfigured (fail-closed, as today).
- Round-trip against a real MongoDB (testcontainers) confirming a sibling field
  not in the Go struct survives the update.

## Migration / compatibility

Pre-release, so no external compatibility constraint, but this is a **behavior
change** for every caller of `ConvertToUpdateDocument` that relied on nested
whole-object replacement. Land as its own PR with:

- an explicit note in `data/mongo/README.md` / `doc.go`,
- the slice-of-structs asymmetry documented,
- benchmarks (`*_bench_test.go`) — dotted-path building adds string work on the
  hot path; measure vs. the current single-assign.

## Rollout

1. Implement behind the recursion `prefix` plumbing; keep slices as whole-array.
2. Full unit + CSFLE + testcontainers coverage.
3. Update docs; remove this file (or mark "implemented in <commit>").
