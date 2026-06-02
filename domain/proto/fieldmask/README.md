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

## Request extraction

Reflection helpers that locate the conventional AIP-134 `update_mask` and AIP-157 `read_mask` fields on a gRPC request message. They look at
descriptors only and do not import any gRPC types — non-gRPC callers can reuse them. The transport-layer wrapper lives in
[`transport/grpc/interceptors/fieldmask`](../../../transport/grpc/interceptors/fieldmask/README.md).

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
