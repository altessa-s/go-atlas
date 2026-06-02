# fieldmask

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
```

Server-side gRPC interceptor that wires [`domain/proto/fieldmask`](../../../../domain/proto/fieldmask/README.md) into the chain: it runs
`FieldMask.ApplyUpdateMask` on AIP-134 `Update*` requests and `FieldMask.Filter` on AIP-157 `Get*` / `List*` / `Search*` / `BatchGet*`
responses, all driven by the gRPC method name. Behavior violations (REQUIRED cleared, IMMUTABLE modified, IDENTIFIER touched) are surfaced as
the canonical `InvalidArgument` + `google.rpc.BadRequest` shape rather than leaking to the storage layer.

## Method classification

The short method name (the part after the final `/`) is matched case-sensitively against AIP prefixes:

| Prefix                                  | Kind         | Effect                                                                |
|-----------------------------------------|--------------|-----------------------------------------------------------------------|
| `Update*`, `Patch*`, `BatchUpdate*`     | `KindUpdate` | `PreCall`: `ApplyUpdateMask(req)`, then writeback the cleaned mask    |
| `Get*`, `List*`, `Search*`, `BatchGet*` | `KindRead`   | `PreCall` captures `read_mask`; `PostCall` runs `Filter(response)`    |
| anything else                           | `KindNone`   | request and response untouched                                        |

`Create*` is deliberately not handled — AIP-133 does not define a mask on the create path. Use `WithMethodKind` to override the
classification for a specific fully-qualified method.

## Options

| Option                                  | Default                          | Description                                                                |
|-----------------------------------------|----------------------------------|----------------------------------------------------------------------------|
| `WithMethodKind(fullMethod, kind)`      | --                               | Override classification for `info.FullMethod`. Accumulates across calls.   |
| `WithSkipReadMask()`                    | off                              | Disable the response-side `Filter` pass on `KindRead` methods.             |
| `WithMaskFieldName(name)`               | `"update_mask"` / `"read_mask"`  | Override the conventional mask field name (descriptor name).               |
| `WithResourceFieldName(name)`           | first non-mask message field     | Override the resource carrier field name on Update requests.               |
| `WithUpdateExtractor(fullMethod, fn)`   | --                               | Per-method update extractor. See [Custom extractors](#custom-extractors).  |
| `WithReadExtractor(fullMethod, fn)`     | --                               | Per-method read extractor.                                                 |
| `WithDefaultUpdateExtractor(fn)`        | built-in reflection              | Global default update extractor used when no per-method override matches.  |
| `WithDefaultReadExtractor(fn)`          | built-in chain                   | Global default read extractor.                                             |
| `WithMetadataReadMaskHeader(name)`      | `"x-goog-fieldmask"`             | Override the gRPC metadata key the built-in AIP-157 read extractor reads.  |
| `WithApplyEmptyUpdateMask()`            | off                              | AIP-134: when `update_mask` is present but empty, synthesise from `FromSetFields(resource)` and apply normally. |
| `WithIgnoreMethods(...string)`          | --                               | Fully-qualified method names to bypass entirely.                           |
| `WithIgnorePatterns(...*regexp.Regexp)` | `defaults.IgnorePatterns`        | Regex patterns to bypass (default skips reflection and health probes).     |
| `WithLogger(*slog.Logger)`              | discard                          | Logger for debug/error messages.                                           |

## Custom extractors

The built-in extractor assumes the canonical AIP-134 shape — `UpdateXxxRequest{Xxx resource, FieldMask update_mask}` — and finds the mask on the
top level of the request via descriptor reflection. Requests that nest the mask inside an `Options` sub-message
(`req.Options.UpdateMask`) are not reachable from there.

For those, plug a typed extractor in. The function types and the generic builders live in
[`domain/proto/fieldmask`](../../../../domain/proto/fieldmask/README.md#custom-extractors) so non-gRPC callers can reuse them.

```go
chain := interceptors.NewChain(
    // ... metadata, auth, fieldbehavior, ...
    fieldmask.ServerInterceptor(
        fieldmask.WithMethodKind("/x.v1.X/UpdateBucket", fieldmask.KindUpdate),
        fieldmask.WithUpdateExtractor("/x.v1.X/UpdateBucket",
            pbfieldmask.NewUpdateExtractor(
                func(r *pb.UpdateBucketRequest) *fieldmaskpb.FieldMask { return r.GetOptions().GetUpdateMask() },
                func(r *pb.UpdateBucketRequest) *pb.Bucket             { return r.GetBucket() },
                func(r *pb.UpdateBucketRequest, m *fieldmaskpb.FieldMask) { r.Options.UpdateMask = m },
            ),
        ),
    ),
    // ... protovalidator, handler ...
)
```

### Resolution precedence

For every call the interceptor resolves the extractor in this order:

1. **Per-method override** registered via `WithUpdateExtractor(fullMethod, fn)` / `WithReadExtractor(fullMethod, fn)` — wins when `fn != nil`.
2. **Global default** registered via `WithDefaultUpdateExtractor(fn)` / `WithDefaultReadExtractor(fn)` — used when no per-method match.
3. **Built-in** — the bottom-of-stack fallback.
   - Update path: `pbfieldmask.DefaultUpdateExtractor` (single reflection extractor). Honors `WithMaskFieldName` / `WithResourceFieldName`.
   - Read path: `pbfieldmask.ChainReadExtractors(MetadataReadExtractor, DefaultReadExtractor)` — modern AIP-157 metadata header first, deprecated
     AIP-161 request-message `read_mask` second.

A custom extractor returning `ok=false` means "I do not handle this request" — the interceptor logs at Debug and passes through. It does **not**
fall back to a lower tier. This keeps the precedence deterministic when callers register an extractor specifically to disable handling for a method.

### AIP-157 metadata read mask

AIP-161 marks `read_mask` on the request message as **deprecated** and forwards callers to AIP-157, which transports the mask through a side
channel: the gRPC metadata key `x-goog-fieldmask` (and the corresponding HTTP `$fields` query parameter, mapped by grpc-gateway). The built-in
read chain consults that header first and falls back to the request-message `read_mask` when the header is absent. Explicit `"*"` and missing
header both mean "all fields" per AIP-157 — the interceptor passes the response through unfiltered.

Override the header name with `WithMetadataReadMaskHeader("x-custom-fieldmask")`. The override applies only to the built-in chain; callers
that replace the read extractor wholesale via `WithDefaultReadExtractor` or `WithReadExtractor` own their own header convention.

### Writeback

`UpdateExtractorFunc` returns an optional writeback closure. The interceptor calls it with the mask after `ApplyUpdateMask` removes OUTPUT_ONLY
entries — for the nested example above, that updates `req.Options.UpdateMask` so the handler sees a coherent `(mask, resource)` pair. Return `nil`
from the writeback slot to skip writeback (when the handler already reads the original mask and ignores OUTPUT_ONLY paths).

## Usage

```go
import (
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
)

server := grpc.NewServer(grpc.UnaryInterceptor(
    interceptors.Chain(
        // ... metadata, auth, fieldbehavior, ...
        fieldmask.ServerInterceptor(
            fieldmask.WithMethodKind("/x.v1.X/Archive", fieldmask.KindUpdate),
        ),
        // ... protovalidator, handler ...
    ),
))
```

`fieldmask` declares dependencies on `metadata` and `auth` — both must run earlier in the chain. Place `fieldbehavior` (and any payload
sanitisation) before `fieldmask` so the mask sees the already-stripped request, and place `protovalidator` after `fieldmask` so the
validator runs against the final mask-cleaned payload.

## Errors

| Source                                            | gRPC status        | Detail attached                          |
|---------------------------------------------------|--------------------|------------------------------------------|
| `*fieldmask.BehaviorViolationError`               | `InvalidArgument`  | `google.rpc.BadRequest.FieldViolation`s  |
| `*fieldmask.ValidationError` (bad mask path)      | `InvalidArgument`  | --                                       |
| Reflection miss (no `update_mask` / `read_mask`)  | --                 | silent passthrough, logged at Debug      |
| Panic inside `ApplyUpdateMask` / `Filter`         | `Internal`         | logged at Error, request rejected        |

Each behavior violation carries the dot-separated field path and a human description (`"immutable field cannot be modified"`,
`"identifier field cannot be modified"`, `"required field cannot be cleared"`).

## Streaming

`PostMsgReceive` / `PostMsgSent` mirror the unary path so the same per-method classification applies to every message in a streaming RPC.
Classification is computed once at stream start from `info.FullMethod`; the `read_mask` captured on the first frame is reused for every
subsequent response frame.

## See also

- [`domain/proto/fieldmask`](../../../../domain/proto/fieldmask/README.md) — the underlying `FieldMask` operations and the
  `ExtractUpdateMask` / `ExtractReadMask` / `SetUpdateMask` reflection helpers this interceptor uses.
- [`docs/domain/proto/fieldmask.md`](../../../../docs/domain/proto/fieldmask.md) — full reference, traversal semantics, performance.
- [`transport/grpc/interceptors/fieldbehavior`](../fieldbehavior/README.md) — sibling AIP-203 interceptor; pair before `fieldmask`.
- [`transport/grpc/interceptors/protovalidator`](../protovalidator/README.md) — pair after `fieldmask` so validation runs on the
  mask-cleaned payload.
