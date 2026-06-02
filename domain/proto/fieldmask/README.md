# fieldmask

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldmask"
```

Package `fieldmask` provides hierarchical field mask utilities for Protocol Buffers. A `FieldMask` is a nested map representing
dot-separated field paths with support for filtering, pruning, set operations, and update-style validation of field annotations.
Well-known types (`structpb.Struct`, `structpb.ListValue`, `structpb.Value`) are handled transparently — their dynamic keys are
navigable by all mask operations. Update masks validate `google.api.field_behavior` annotations (`REQUIRED`, `IMMUTABLE`,
`OUTPUT_ONLY`, `IDENTIFIER`) before applying changes. `IDENTIFIER` fields (AIP-203) are treated like `IMMUTABLE` in the update
path — the identifier names the resource and must not be modified by an update.

## Constructors

| Function               | Description                                                                                            |
|------------------------|--------------------------------------------------------------------------------------------------------|
| `FromPaths`            | Create a mask from variadic dot-separated path strings                                                 |
| `FromProtoFieldMask`   | Create a mask from a `fieldmaskpb.FieldMask` proto message                                            |
| `FromMessage`          | Create a mask covering every field in the message schema                                               |
| `FromSetFields`        | Create a mask covering only the populated (non-default) fields of a message                            |

## Operations

| Method                 | Description                                                                                            |
|------------------------|--------------------------------------------------------------------------------------------------------|
| `Filter`               | Keep only the fields present in the mask, clearing everything else                                     |
| `Prune`                | Remove the fields present in the mask, keeping everything else                                         |
| `Union`                | Combine two masks into one that covers both sets of paths                                              |
| `Intersection`         | Return a mask containing only the paths present in both masks                                          |
| `Difference`           | Return a mask containing paths in the receiver but not in the argument                                 |
| `ToPaths`              | Return a sorted slice of dot-separated path strings                                                    |
| `ApplyUpdateMask`      | Copy masked fields from source to destination with field behavior validation                           |

`ApplyUpdateMask` also rejects indexed access into repeated fields (`authors.0`, `authors.0.given_name`) as `ValidationError` per AIP-161:
update masks address whole repeated fields, never a single element. To replace one entry, callers replace the entire list. Read paths
(`Filter`, `Prune`, `Validate`) tolerate the same segments — AIP-161 lets the implementation ignore them on read.

## Request extraction

Read the `update_mask` and the read mask off a gRPC request. There are three transports; all of them return `*fieldmaskpb.FieldMask`.

| AIP                          | Transport                                                                | Helper                                              |
|------------------------------|--------------------------------------------------------------------------|-----------------------------------------------------|
| AIP-134                      | `update_mask` field on the request message                               | `ExtractUpdateMask` / `DefaultUpdateExtractor`      |
| AIP-157 (modern)             | gRPC metadata `x-goog-fieldmask` (or HTTP `$fields` mapped by gateway)   | `MetadataReadExtractor`                             |
| AIP-157 via AIP-161 (legacy) | `read_mask` field on the request message — deprecated by AIP-161         | `ExtractReadMask` / `DefaultReadExtractor`          |

Reflection helpers stay gRPC-unaware (descriptors only); the metadata helper reads `metadata.FromIncomingContext`. All three work from non-gRPC
callers too. The interceptor in [`transport/grpc/interceptors/fieldmask`](../../../transport/grpc/interceptors/fieldmask/README.md) chains
`MetadataReadExtractor → DefaultReadExtractor` by default, so the modern path is preferred and the deprecated path keeps working.

### Reflection helpers

| Function                                          | Description                                                                                          |
|---------------------------------------------------|------------------------------------------------------------------------------------------------------|
| `ExtractUpdateMask(req, opts...)`                 | Return the `update_mask` and the sibling resource sub-message. Defaults to `"update_mask"` and the first non-mask message field. |
| `ExtractReadMask(req, opts...)`                   | Return the `read_mask` on a Get/List/Search request. Defaults to `"read_mask"`.                      |
| `SetUpdateMask(req, mask, opts...)`               | Writeback helper used after `ApplyUpdateMask` removes `OUTPUT_ONLY` entries. Pass `nil` to clear.    |
| `WithMaskField(name)` / `WithResourceField(name)` | Override the conventional field names (descriptor name, not Go name).                                |

`SetUpdateMask` returns `ErrFieldNotSettable` when the request does not expose an `update_mask` field of type `google.protobuf.FieldMask`.

### Custom extractors

For requests that do not follow the AIP-134 flat shape — most commonly when the mask lives inside an `Options` sub-message
(`req.Options.UpdateMask`) — the reflection helpers above will not find the mask. Use the function-type API instead and plug a typed extractor into
the gRPC interceptor via `WithUpdateExtractor` / `WithReadExtractor`.

| Symbol                                                                                                                   | Purpose                                                                                                                                |
|--------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| `UpdateExtractorFunc`                                                                                                    | `func(ctx, req) (mask, resource, writeback, ok)` — transport-agnostic shape consumed by the gRPC interceptor.                          |
| `ReadExtractorFunc`                                                                                                      | `func(ctx, req) (mask, ok)`.                                                                                                           |
| `NewUpdateExtractor[ReqT, ResT proto.Message](getMask, getResource, setMask)`                                            | Generic builder over typed getters. Returns ok=false on type mismatch, nil mask, or typed-nil resource. `setMask` may be nil.          |
| `NewReadExtractor[ReqT proto.Message](getMask)`                                                                          | Generic builder for read masks.                                                                                                        |
| `DefaultUpdateExtractor(opts ...ExtractOption)` / `DefaultReadExtractor(opts ...ExtractOption)`                          | Wrap the reflection helpers above in the new function shape. Used by the gRPC interceptor as the bottom-of-stack fallback.             |

```go
extract := fieldmask.NewUpdateExtractor(
    func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
    func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
    func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
)
```

### Metadata-based read mask (AIP-157)

AIP-161 deprecates the request-message `read_mask` and routes callers through AIP-157, which moves the mask onto a side channel — the
`x-goog-fieldmask` gRPC metadata key in Google Cloud (and the matching `$fields` HTTP query, mapped by grpc-gateway). `MetadataReadExtractor`
reads that header.

| Symbol                                       | Purpose                                                                                                                                |
|----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| `MetadataReadExtractor(opts ...MetadataExtractorOption)` | Reads the field mask from incoming gRPC metadata per AIP-157.                                                                          |
| `WithMetadataHeader(name)`                   | Override the metadata key. Defaults to `DefaultMetadataReadMaskHeader` (`"x-goog-fieldmask"`).                                         |
| `DefaultMetadataReadMaskHeader`              | Constant for the default header name.                                                                                                  |

Semantics:

| Metadata state                              | Result                                                |
|---------------------------------------------|-------------------------------------------------------|
| No incoming metadata in `ctx`               | `ok=false` (silent passthrough)                       |
| Header absent                               | `ok=false`                                            |
| Header empty / whitespace only              | `ok=false` (AIP-157: omitted ⇒ all fields)            |
| Header == `"*"`                             | `ok=false` (AIP-157: `*` ⇒ all fields)                |
| Header == `"name,authors.given_name"`       | `&fieldmaskpb.FieldMask{Paths: [name, …]}, true`      |
| Multiple values on the same key             | First value wins; the rest are ignored                |
| Surrounding whitespace around paths         | Trimmed; empty entries dropped                        |

```go
extract := fieldmask.MetadataReadExtractor(fieldmask.WithMetadataHeader("x-custom-fieldmask"))
```

### Chaining extractors

`ChainReadExtractors` and `ChainUpdateExtractors` combine extractors and return the first one that signals `ok=true`. Nil entries are
skipped; an empty chain returns `nil`.

| Symbol                          | Purpose                                                                                                       |
|---------------------------------|---------------------------------------------------------------------------------------------------------------|
| `ChainReadExtractors(fns ...)`  | First-ok-wins composition over `ReadExtractorFunc`. Used by the gRPC interceptor to combine metadata + legacy request-field paths. |
| `ChainUpdateExtractors(fns ...)`| Same for `UpdateExtractorFunc`. Exposed for per-service composition; the gRPC interceptor's update path is a single extractor because AIP-134 mandates `update_mask` on the request message. |

```go
read := fieldmask.ChainReadExtractors(
    fieldmask.MetadataReadExtractor(),        // AIP-157 modern path
    fieldmask.DefaultReadExtractor(),         // AIP-161 deprecated request-field fallback
)
```
