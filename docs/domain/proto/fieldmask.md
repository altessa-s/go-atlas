# Field Mask gRPC interceptor (`transport/grpc/interceptors/fieldmask`)

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
```

> The interceptor lives under `transport/grpc/interceptors/`. It is documented here alongside the data-layer guides because it sits directly on the
> request/response data path and is the canonical gRPC wiring for the `domain/proto/fieldmask` package.

A server interceptor that wires [`domain/proto/fieldmask`](../../../domain/proto/fieldmask/README.md) into the gRPC chain: it runs
`FieldMask.ApplyUpdateMask` on AIP-134 `Update*` requests and `FieldMask.Filter` on AIP-157 `Get*`/`List*`/`Search*`/`BatchGet*` responses based on the
gRPC method name. Behavior violations (REQUIRED cleared, IMMUTABLE modified, IDENTIFIER touched) leave the storage layer as `InvalidArgument` with a
`google.rpc.BadRequest` detail rather than as a 500 deep inside a repository call.

The companion package [`fieldbehavior`](./fieldbehavior.md) handles the resource *payload*; this package handles the `FieldMask` *value*. You almost
always want both — `fieldbehavior` before `fieldmask` so the mask sees the already-stripped resource.

---

## When to reach for the interceptor

| Scenario                                                                                  | Use                                                |
|-------------------------------------------------------------------------------------------|----------------------------------------------------|
| Apply AIP-134 update semantics for every `Update*` method                                 | Add to `interceptors.Chain` — no per-handler code  |
| Apply AIP-157 read-mask projection for every `Get*` / `List*` / `Search*` method          | Same — `read_mask` filter runs in `PostCall`       |
| Reject IMMUTABLE / IDENTIFIER modifications at the wire boundary, with `FieldViolation`s  | Default behavior of `KindUpdate`                   |
| Strip `OUTPUT_ONLY` entries from the writeback `update_mask` before the handler sees it   | Default behavior of `KindUpdate`                   |
| Opt a method out of the interceptor without removing it from the chain                    | `WithMethodKind(method, KindSkip)`                 |
| Custom mask field name on a non-AIP RPC shape                                             | `WithMaskFieldName`, `WithResourceFieldName`       |

---

## Method classification

The short method name (the part after the final `/`) is matched case-sensitively against AIP prefixes:

| Prefix                                  | Kind         | Effect                                                                |
|-----------------------------------------|--------------|-----------------------------------------------------------------------|
| `Update*`, `Patch*`, `BatchUpdate*`     | `KindUpdate` | `PreCall`: `ApplyUpdateMask(req)`, then writeback the cleaned mask    |
| `Get*`, `List*`, `Search*`, `BatchGet*` | `KindRead`   | `PreCall` captures `read_mask`; `PostCall` runs `Filter(response)`    |
| anything else                           | `KindNone`   | request and response untouched                                        |

`Create*` is deliberately not handled — AIP-133 does not define a mask on the create path. Use `WithMethodKind` for non-AIP methods (e.g. an `Archive`
RPC that semantically updates a resource).

---

## Quick start

```go
import (
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
)

chain := interceptors.NewChain(
    // ... metadata, auth, fieldbehavior, ...
    fieldmask.ServerInterceptor(
        fieldmask.WithMethodKind("/x.v1.X/Archive", fieldmask.KindUpdate),
    ),
    // ... protovalidator, handler ...
)
```

`fieldmask` declares dependencies on `metadata` and `auth` — both run earlier in the chain. Place it after `fieldbehavior` so the resource is already
sanitised by the time `ApplyUpdateMask` reads it; place it before `protovalidator` so validation runs against the mask-cleaned payload.

---

## Update path

For a `KindUpdate` method, on every unary call (and every streamed frame):

1. `pbfieldmask.ExtractUpdateMask(req)` locates the `update_mask` field and the sibling resource sub-message via descriptor reflection.
2. The interceptor builds a `fieldmask.FieldMask` from the raw `update_mask.paths` and calls `ApplyUpdateMask(resource)`.
3. `ApplyUpdateMask` validates field behaviors against the resource descriptor:
   * REQUIRED in the mask but cleared on the resource → violation.
   * IMMUTABLE or IDENTIFIER in the mask → violation (AIP-203).
   * OUTPUT_ONLY in the mask → silently dropped from the mask map.
4. On violation: the interceptor builds an `InvalidArgument` status with a `google.rpc.BadRequest` detail — one `FieldViolation` per entry, each
   carrying the dot-separated field path and a human description.
5. On success: the cleaned mask (OUTPUT_ONLY entries removed) is written back to `req.update_mask` via `SetUpdateMask`, so the handler sees a
   coherent `(mask, resource)` pair.

The handler can then treat `req.GetUpdateMask().GetPaths()` as a clean instruction list — no IMMUTABLE/IDENTIFIER booby traps, no OUTPUT_ONLY
writes to discard at the storage layer.

---

## Read path

For a `KindRead` method:

1. `PreCall` extracts the `read_mask` from the request via `pbfieldmask.ExtractReadMask`. Absence is a silent passthrough.
2. The handler runs against the unmodified request.
3. `PostCall` builds a `FieldMask` from the captured `read_mask` and calls `Filter(response)` — fields outside the mask are cleared before the
   response goes back to the wire.
4. Handler errors short-circuit the filter — gRPC drops the response body anyway.

`WithSkipReadMask()` disables the `PostCall` pass when the handler already applies the read mask itself (e.g. when filtering at the database layer
saves a CPU pass and the response is already minimal).

---

## Options

| Option                                  | Default                          | Description                                                                |
|-----------------------------------------|----------------------------------|----------------------------------------------------------------------------|
| `WithMethodKind(fullMethod, kind)`      | --                               | Override classification for `info.FullMethod`. Accumulates across calls.   |
| `WithSkipReadMask()`                    | off                              | Disable the response-side `Filter` pass on `KindRead` methods.             |
| `WithMaskFieldName(name)`               | `"update_mask"` / `"read_mask"`  | Override the conventional mask field name (descriptor name).               |
| `WithResourceFieldName(name)`           | first non-mask message field     | Override the resource carrier field name on Update requests.               |
| `WithIgnoreMethods(...string)`          | --                               | Fully-qualified method names to bypass entirely.                           |
| `WithIgnorePatterns(...*regexp.Regexp)` | `defaults.IgnorePatterns`        | Regex patterns to bypass (default skips reflection and health probes).     |
| `WithLogger(*slog.Logger)`              | discard                          | Logger for debug/error messages.                                           |

---

## Errors

| Source                                            | gRPC status        | Detail attached                          |
|---------------------------------------------------|--------------------|------------------------------------------|
| `*fieldmask.BehaviorViolationError`               | `InvalidArgument`  | `google.rpc.BadRequest.FieldViolation`s  |
| `*fieldmask.ValidationError` (bad mask path)      | `InvalidArgument`  | --                                       |
| Reflection miss (no `update_mask` / `read_mask`)  | --                 | silent passthrough, logged at Debug      |
| Panic inside `ApplyUpdateMask` / `Filter`         | `Internal`         | logged at Error, request rejected        |

Behavior violations carry the AIP-canonical shape; client libraries that already know how to surface `google.rpc.BadRequest` (Google Cloud SDKs,
grpc-gateway with `errdetails` support, the official Java/Python/Go SDKs) get a structured per-field error list without any handler glue.

---

## Request extraction

Three transports, one return type (`*fieldmaskpb.FieldMask`):

| AIP                                          | Transport                                                              | Helper (in `domain/proto/fieldmask`)                |
|----------------------------------------------|------------------------------------------------------------------------|-----------------------------------------------------|
| AIP-134 (Standard Update)                    | `update_mask` field on the request message                             | `ExtractUpdateMask` / `DefaultUpdateExtractor`      |
| AIP-157 (Partial responses) — modern         | gRPC metadata `x-goog-fieldmask` (or HTTP `$fields` via grpc-gateway)  | `MetadataReadExtractor`                             |
| AIP-157 via AIP-161 — legacy                 | `read_mask` field on the request message — AIP-161 deprecates this     | `ExtractReadMask` / `DefaultReadExtractor`          |

Reflection helpers stay gRPC-unaware (descriptors only); the metadata helper reads `metadata.FromIncomingContext`. All three work from non-gRPC
callers — anywhere you have a `proto.Message`.

| Function                              | Purpose                                                                                          |
|---------------------------------------|--------------------------------------------------------------------------------------------------|
| `ExtractUpdateMask(req, opts...)`     | Locate the `update_mask` field and the sibling resource sub-message via descriptor reflection.   |
| `ExtractReadMask(req, opts...)`       | Locate the `read_mask` field on a Get/List/Search request.                                       |
| `SetUpdateMask(req, mask, opts...)`   | Writeback the cleaned mask onto the request. Returns `ErrFieldNotSettable` on shape mismatch.    |
| `MetadataReadExtractor(opts...)`      | Read the mask from gRPC metadata per AIP-157. Default key `x-goog-fieldmask`.                    |

---

## Custom extractors

The built-in reflection extractor assumes the canonical AIP-134 shape — `UpdateXxxRequest{Xxx resource, FieldMask update_mask}` — and locates the
mask on the top level of the request. Requests that nest the mask inside an `Options` sub-message (`req.Options.UpdateMask`) are not reachable from
there. Plug in a typed extractor instead.

The function types and the generic builders live in `domain/proto/fieldmask` so non-gRPC callers (HTTP handlers, batch jobs) can reuse them:

| Symbol                                                                          | Purpose                                                                                                              |
|---------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `UpdateExtractorFunc`                                                           | `func(ctx, req) (mask, resource, writeback, ok)`. Returning `ok=false` is a silent passthrough.                      |
| `ReadExtractorFunc`                                                             | `func(ctx, req) (mask, ok)`.                                                                                         |
| `NewUpdateExtractor[ReqT, ResT proto.Message](getMask, getResource, setMask)`   | Generic builder over typed getters. `setMask` may be nil to skip writeback.                                          |
| `NewReadExtractor[ReqT proto.Message](getMask)`                                 | Generic builder for read masks.                                                                                      |
| `DefaultUpdateExtractor(opts ...ExtractOption)` / `DefaultReadExtractor(...)`   | Wrap the reflection helpers above in the `UpdateExtractorFunc` / `ReadExtractorFunc` shape.                          |
| `MetadataReadExtractor(opts ...MetadataExtractorOption)`                        | Read the field mask from gRPC metadata per AIP-157. Defaults to the `x-goog-fieldmask` key.                          |
| `WithMetadataHeader(name)`                                                      | Override the metadata key consulted by `MetadataReadExtractor`.                                                      |
| `ChainReadExtractors(fns ...)` / `ChainUpdateExtractors(fns ...)`               | First-ok-wins composition. The built-in read fallback is a chain over `MetadataReadExtractor` + `DefaultReadExtractor`. |

Register the extractor on the gRPC interceptor:

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
3. **Built-in** — the bottom-of-stack fallback. The update path stays a single `DefaultUpdateExtractor`; the read path is
   `ChainReadExtractors(MetadataReadExtractor, DefaultReadExtractor)`. Honors `WithMaskFieldName` / `WithResourceFieldName`.

A custom extractor returning `ok=false` means "I do not handle this request" — the interceptor logs at Debug and passes through. It does **not**
fall back to a lower tier. The semantics are deterministic by design: a caller who registers an extractor specifically to opt a method out of
handling gets that behavior, not a quiet hand-off to reflection.

### AIP-157 metadata read mask

AIP-161 marks `read_mask` on the request message as **deprecated** and routes callers through AIP-157, which carries the mask via a side channel:
the `x-goog-fieldmask` gRPC metadata key (and the `$fields` HTTP query parameter that grpc-gateway maps onto it). The built-in read chain reads
that header first and falls back to the request-message `read_mask` only when the header is absent. Both the explicit `"*"` value and a missing
header mean "all fields" — the interceptor passes the response through unfiltered.

Override the header name with `WithMetadataReadMaskHeader("x-custom-fieldmask")` on the gRPC interceptor. The override only affects the built-in
chain; callers that replace the read extractor wholesale own their own header convention.

### Writeback

`UpdateExtractorFunc` returns an optional writeback closure. The interceptor calls it with the cleaned mask after `ApplyUpdateMask` removes
OUTPUT_ONLY entries — for the nested example above, that updates `req.Options.UpdateMask` so the handler sees a coherent `(mask, resource)` pair.
Return `nil` from the writeback slot to skip writeback (e.g. when the handler reads the original mask and ignores OUTPUT_ONLY paths).

---

## See also

* [`domain/proto/fieldmask`](../../../domain/proto/fieldmask/README.md) — the underlying `FieldMask` operations.
* [`fieldbehavior.md`](./fieldbehavior.md) — sibling AIP-203 payload stripper; pair with `fieldmask` for full AIP-133/134/157 wire semantics.
* [`transport/grpc/interceptors/fieldmask`](../../../transport/grpc/interceptors/fieldmask/README.md) — package-level reference.
* **AIP-134** — Standard Update method ([aip.dev/134](https://google.aip.dev/134)).
* **AIP-157** — Partial responses with `read_mask` ([aip.dev/157](https://google.aip.dev/157)).
* **AIP-203** — `field_behavior` annotations ([aip.dev/203](https://google.aip.dev/203)).
