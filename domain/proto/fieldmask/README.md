# fieldmask

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
```

Package `fieldmask` provides hierarchical field mask utilities for Protocol Buffers. A `FieldMask` is a nested map representing dot-separated field
paths with support for filtering, pruning, set operations, and update-style validation of field annotations. Well-known types (`structpb.Struct`,
`structpb.ListValue`, `structpb.Value`) are handled transparently — their dynamic keys are navigable by all mask operations. Update masks validate
`google.api.field_behavior` annotations (`REQUIRED`, `IMMUTABLE`, `OUTPUT_ONLY`, `IDENTIFIER`) before applying changes. `IDENTIFIER` fields (AIP-203)
are treated like `IMMUTABLE` in the update path — the identifier names the resource and must not be modified by an update. A singular message named
whole (`profile`) would be replaced or cleared together with its fields, so when it holds an `IDENTIFIER`, `IMMUTABLE` or `OUTPUT_ONLY` field — directly
or in a singular sub-message at any depth, populated or absent — each such field is reported as a violation; name the message's other fields
(`profile.display_name`) instead. Repeated and map fields, and their elements named whole (`labels.k`, `aliases.*`), are replaced as a whole and are not
descended. A sub-path into collection elements — a map key (`labels.k.id`), the `*` wildcard (`aliases.*.id`) or the implicit per-element form
(`aliases.id`) — is checked against the element schema: `IDENTIFIER` and `IMMUTABLE` leaves are violations and `OUTPUT_ONLY` leaves are removed from the
mask, as for a singular sub-message. Each element such a sub-path addresses keeps its set `IDENTIFIER` fields, as a singular sub-message does:
`labels.k.display_name` adds `labels.k.id` to the mask so the filter keeps it, and `DropIdentifiers` removes it again.

A field carrying several behaviors is judged by presence with a fixed precedence — `OUTPUT_ONLY` (dropped), then `IDENTIFIER`, then `IMMUTABLE`, then
`REQUIRED` (cleared) — so `[REQUIRED, IMMUTABLE]` is protected whatever the declaration order, and the violation names the protecting behavior. A
whole-message selection is rejected when its singular subtree holds any server-managed field — `OUTPUT_ONLY`, `IDENTIFIER` or `IMMUTABLE`: the
replacement (for example `FieldMask.Merge`, which copies whole top-level fields) would delete the stored value. The violation names `OUTPUT_ONLY` for a
field that is also `IDENTIFIER` or `IMMUTABLE`. Nested `OUTPUT_ONLY` entries are removed from the mask before set identifiers are preserved (an
`IDENTIFIER` that is also `OUTPUT_ONLY` is never preserved), so a branch that named only such fields disappears.

## Constructors

| Function                 | Description                                                                                                     |
|--------------------------|-----------------------------------------------------------------------------------------------------------------|
| `FromPaths`              | Create a mask from variadic dot-separated path strings                                                          |
| `FromProtoFieldMask`     | Create a mask from a `fieldmaskpb.FieldMask` proto message                                                      |
| `FromMessage`            | Create a mask covering every field in the message schema                                                        |
| `FromSetFields`          | Create a mask covering only the populated (non-default) fields of a message; options shape the result           |
| `FromSetFieldsForUpdate` | The update mask an empty `update_mask` implies: `FromSetFields` minus identifiers and server-managed sub-fields |

## Operations

| Method               | Description                                                                                        |
|----------------------|----------------------------------------------------------------------------------------------------|
| `Filter`             | Keep only the fields present in the mask, clearing everything else; an empty mask keeps everything |
| `Select`             | Like `Filter`, but an empty mask clears everything: use it for masks computed with exclusions      |
| `Prune`              | Remove the fields present in the mask, keeping everything else                                     |
| `PrunePaths`         | Package-level shorthand: prune the given paths from a message without building a mask first        |
| `Union`              | Combine two masks into one that covers both sets of paths                                          |
| `Intersection`       | Return a mask containing only the paths present in both masks                                      |
| `Difference`         | Return a mask containing paths in the receiver but not in the argument                             |
| `ToPaths`            | Return a sorted slice of dot-separated path strings                                                |
| `ApplyUpdateMask`    | Validate field behaviors, then clear fields outside the mask in place (mutates the receiver mask)  |
| `DropIdentifiers`    | Remove the `IDENTIFIER` entries `ApplyUpdateMask` added, before handing the mask on (mutates)      |
| `ExpandMessagePaths` | Expand message-typed leaves into their sub-fields for a full replacement update                    |
| `Contains`           | Report whether a dot-separated path is present in the mask                                         |
| `Validate`           | Check that every path exists in the message schema (`ValidationError` otherwise)                   |
| `Clone`              | Deep copy of the mask                                                                              |
| `ToProtoFieldMask`   | Convert the mask to a `fieldmaskpb.FieldMask`                                                      |

`ApplyUpdateMask` also rejects indexed access into repeated fields (`authors.0`, `authors.0.given_name`) as `ValidationError` per AIP-161: update masks
address whole repeated fields, never a single element. To replace one entry, callers replace the entire list. Read paths (`Filter`, `Prune`, `Validate`)
tolerate the same segments — AIP-161 lets the implementation ignore them on read.

By default `ApplyUpdateMask` silently ignores paths absent from the message schema. `WithPathValidation` selects how those paths are treated (AIP-161),
running `Validate` before any mutation:

| Mode                     | Behavior                                                                                     |
|--------------------------|----------------------------------------------------------------------------------------------|
| `PathValidationDisabled` | Default. Ignore schema-invalid paths — historical behavior.                                  |
| `PathValidationWarn`     | Report the bad path to `WithPathValidationReporter` and still apply the mask.                |
| `PathValidationEnforce`  | Reject with `ValidationError` before any mutation, so a typo cannot clear a sibling subtree. |

```go
// Fail fast on unknown paths.
err := mask.ApplyUpdateMask(msg, fieldmask.WithPathValidation(fieldmask.PathValidationEnforce))

// Or surface them without breaking callers while a fix rolls out.
err = mask.ApplyUpdateMask(msg,
    fieldmask.WithPathValidation(fieldmask.PathValidationWarn),
    fieldmask.WithPathValidationReporter(func(ve *fieldmask.ValidationError) { log.Warn("bad mask path", "path", ve.Path) }),
)
```

### Backtick-quoted map keys (AIP-161)

Map keys that contain a dot or other non-identifier characters are wrapped in backticks so the dot is not mistaken for a path separator. The parser
(`FromPaths`, `Contains`, `Validate`) treats a backtick-quoted run as a single segment and strips the backticks; `ToPaths` re-quotes any key that still
contains a dot, so the round-trip is lossless.

```go
mask := fieldmask.FromPaths(
    "reviews.`John Smith`.score",       // key has a space, no dot
    "metadata.`google.com/project`",    // key has a dot — backticks survive ToPaths
)

mask.Contains("reviews.`John Smith`.score") // true
mask.ToPaths()                              // ["metadata.`google.com/project`", "reviews.John Smith.score"]
```

The same syntax is honored by `Filter`, `Prune`, and `ApplyUpdateMask` when traversing `map<string, …>` fields.

### Wildcard segments (AIP-161)

The `*` segment (constant `WildcardSegment`) matches every element of a repeated field or every value of a map field. It is honored by `Filter`,
`Prune`, `Validate`, and `ApplyUpdateMask`. A leaf wildcard keeps (or, for `Prune`, clears) the whole collection; a branch wildcard applies the nested
sub-mask to every element or value. For maps, a specific-key entry wins over the wildcard for matched keys.

```go
fieldmask.FromPaths("aliases.*.display_name")   // keep display_name on every alias
fieldmask.FromPaths("labels.*")                 // leaf wildcard — Filter keeps every entry, Prune clears them all

mask := fieldmask.FromPaths(
    "labels.admin.id",       // specific key wins for "admin"
    "labels.*.display_name", // wildcard governs the rest
)
```

`ApplyUpdateMask` strips OUTPUT_ONLY fields transitively through element sub-masks: `aliases.*.updated_at`, `aliases.updated_at` and
`labels.k.updated_at` (where `updated_at` is OUTPUT_ONLY on `Profile`) are removed from the writeback mask before the handler sees it. When an element
sub-mask becomes empty after the strip, it is dropped, and so is a parent collection left without entries.

## Read-modify-write updates

For a handler that applies an update mask itself — typically a read-modify-write of a stored resource — rather than through `ApplyUpdateMask`.

| Symbol                        | Description                                                                                                      |
|-------------------------------|------------------------------------------------------------------------------------------------------------------|
| `FieldMask.SelectsAll()`      | `true` for the full-replacement mask `*`; `*` with other paths is `ErrWildcardNotAlone`                          |
| `FieldMask.Resolve(md)`       | Validates the mask against `md` and spells every field segment by its declared name (aliases below)              |
| `FieldMask.Merge(dst, src)`   | Copies the selected top-level fields from `src` (deep copy; unset in `src` clears `dst`); empty mask = no change |
| `FromSetFields(msg, opts...)` | With `WithTopLevelOnly`, `WithDeclarationOrder`, `WithSkipFields`: the mask an update implies when it is empty   |

`Resolve` accepts every spelling of a field: its declared name (`tenant_id`), its JSON name (`tenantId`), and the snake_case form protojson decodes
a JSON mask path to (a field declared `noisePercent7d` arrives as `noise_percent7d`). Map keys, list indexes and wildcards are kept as written; an
unknown path is a `ValidationError`, as from `Validate`.

`Merge` means exactly what the mask names: `*` alone selects every top-level field, an empty mask changes nothing (for example a mask that named only
a write-only field the handler applies itself), and a nested or unknown path is a `ValidationError`. For the mask an update implies when the request
leaves it empty, build it from the request with `FromSetFields` first. A nested `WithSkipFields` entry (`profile.secret`) never comes back through
a parent that covers it: the parent is replaced by its other set sub-paths (`profile.display_name`, which `Filter` honors and `Merge` rejects as a
nested path), and where that cannot be expressed (`WithTopLevelOnly`, or a path through a repeated, map or `Struct` field) the whole top-level
field is left out.

**Apply a mask computed with `WithSkipFields` with `Select` or `Merge`, never with `Filter`.** When every populated field is skipped the
computed mask is empty; `Filter` reads an empty mask as "keep everything" (and would keep the skipped fields), `Select` clears everything and
`Merge` changes nothing. Nil or typed-nil messages, or messages that are not of one Go type and one
descriptor (equal full names are not enough, and a `dynamicpb` message does not mix with the generated type of its descriptor), are
`ErrMessageMismatch`. Every check runs before the first write, so `dst` is changed all-or-nothing.

```go
mask := fieldmask.FromProtoFieldMask(req.GetUpdateMask())
if mask.IsEmpty() {
    mask = fieldmask.FromSetFields[fieldmask.FieldMask](req.GetBook(), fieldmask.WithSkipFields("name", "etag"))
}
merged := proto.Clone(current)
if err := mask.Merge(merged, req.GetBook()); err != nil {
    return nil, err // ErrWildcardNotAlone, *ValidationError or ErrMessageMismatch
}
```

## Request extraction

Read the `update_mask` and the read mask off a gRPC request. There are three transports; all of them return `*fieldmaskpb.FieldMask`.

| AIP                          | Transport                                                              | Helper                                         |
|------------------------------|------------------------------------------------------------------------|------------------------------------------------|
| AIP-134                      | `update_mask` field on the request message                             | `ExtractUpdateMask` / `DefaultUpdateExtractor` |
| AIP-157 (modern)             | gRPC metadata `x-goog-fieldmask` (or HTTP `$fields` mapped by gateway) | `MetadataReadExtractor`                        |
| AIP-157 via AIP-161 (legacy) | `read_mask` field on the request message — deprecated by AIP-161       | `ExtractReadMask` / `DefaultReadExtractor`     |

Reflection helpers stay gRPC-unaware (descriptors only); the metadata helper reads `metadata.FromIncomingContext`. All three work from non-gRPC callers
too. The interceptor in [`transport/grpc/interceptors/fieldmask`](../../../transport/grpc/interceptors/fieldmask/README.md) chains
`MetadataReadExtractor → DefaultReadExtractor` by default, so the modern path is preferred and the deprecated path keeps working.

### Reflection helpers

| Function                                          | Description                                                                                                                      |
|---------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `ExtractUpdateMask(req, opts...)`                 | Return the `update_mask` and the sibling resource sub-message. Defaults to `"update_mask"` and the first non-mask message field. |
| `ExtractReadMask(req, opts...)`                   | Return the `read_mask` on a Get/List/Search request. Defaults to `"read_mask"`.                                                  |
| `SetUpdateMask(req, mask, opts...)`               | Writeback helper used after `ApplyUpdateMask` removes `OUTPUT_ONLY` entries. Pass `nil` to clear.                                |
| `WithMaskField(name)` / `WithResourceField(name)` | Override the conventional field names (descriptor name, not Go name).                                                            |

`SetUpdateMask` returns `ErrFieldNotSettable` when the request does not expose an `update_mask` field of type `google.protobuf.FieldMask`.

### Custom extractors

For requests that do not follow the AIP-134 flat shape — most commonly when the mask lives inside an `Options` sub-message (`req.Options.UpdateMask`) —
the reflection helpers above will not find the mask. Use the function-type API instead and plug a typed extractor into the gRPC interceptor via
`WithUpdateExtractor` / `WithReadExtractor`.

| Symbol                                                                                          | Purpose                                                                                                                       |
|-------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `UpdateExtractorFunc`                                                                           | `func(ctx, req) (mask, resource, writeback, ok)` — transport-agnostic shape consumed by the gRPC interceptor.                 |
| `ReadExtractorFunc`                                                                             | `func(ctx, req) (mask, ok)`.                                                                                                  |
| `NewUpdateExtractor[ReqT, ResT proto.Message](getMask, getResource, setMask)`                   | Generic builder over typed getters. Returns ok=false on type mismatch, nil mask, or typed-nil resource. `setMask` may be nil. |
| `NewReadExtractor[ReqT proto.Message](getMask)`                                                 | Generic builder for read masks.                                                                                               |
| `DefaultUpdateExtractor(opts ...ExtractOption)` / `DefaultReadExtractor(opts ...ExtractOption)` | Wrap the reflection helpers above in the new function shape. Used by the gRPC interceptor as the bottom-of-stack fallback.    |

```go
extract := fieldmask.NewUpdateExtractor(
    func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
    func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
    func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
)
```

### Metadata-based read mask (AIP-157)

AIP-161 deprecates the request-message `read_mask` and routes callers through AIP-157, which moves the mask onto a side channel — the `x-goog-fieldmask`
gRPC metadata key in Google Cloud (and the matching `$fields` HTTP query, mapped by grpc-gateway). `MetadataReadExtractor` reads that header.

| Symbol                                                   | Purpose                                                                                        |
|----------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `MetadataReadExtractor(opts ...MetadataExtractorOption)` | Reads the field mask from incoming gRPC metadata per AIP-157.                                  |
| `WithMetadataHeader(name)`                               | Override the metadata key. Defaults to `DefaultMetadataReadMaskHeader` (`"x-goog-fieldmask"`). |
| `DefaultMetadataReadMaskHeader`                          | Constant for the default header name.                                                          |

Semantics:

| Metadata state                        | Result                                           |
|---------------------------------------|--------------------------------------------------|
| No incoming metadata in `ctx`         | `ok=false` (silent passthrough)                  |
| Header absent                         | `ok=false`                                       |
| Header empty / whitespace only        | `ok=false` (AIP-157: omitted ⇒ all fields)       |
| Header == `"*"`                       | `ok=false` (AIP-157: `*` ⇒ all fields)           |
| Header == `"name,authors.given_name"` | `&fieldmaskpb.FieldMask{Paths: [name, …]}, true` |
| Multiple values on the same key       | First value wins; the rest are ignored           |
| Surrounding whitespace around paths   | Trimmed; empty entries dropped                   |

```go
extract := fieldmask.MetadataReadExtractor(fieldmask.WithMetadataHeader("x-custom-fieldmask"))
```

### Chaining extractors

`ChainReadExtractors` and `ChainUpdateExtractors` combine extractors and return the first one that signals `ok=true`. Nil entries are skipped; an empty
chain returns `nil`.

| Symbol                           | Purpose                                                                                                                                                                                      |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ChainReadExtractors(fns ...)`   | First-ok-wins composition over `ReadExtractorFunc`. Used by the gRPC interceptor to combine metadata + legacy request-field paths.                                                           |
| `ChainUpdateExtractors(fns ...)` | Same for `UpdateExtractorFunc`. Exposed for per-service composition; the gRPC interceptor's update path is a single extractor because AIP-134 mandates `update_mask` on the request message. |

```go
read := fieldmask.ChainReadExtractors(
    fieldmask.MetadataReadExtractor(),        // AIP-157 modern path
    fieldmask.DefaultReadExtractor(),         // AIP-161 deprecated request-field fallback
)
```
