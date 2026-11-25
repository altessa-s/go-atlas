# fieldmask

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
```

Server-side gRPC interceptor that wires go-aip [`fieldmask`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask) and its [`update`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask/update) and
[`extract`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask/extract) packages into the chain: it applies the `update_mask` of AIP-134 `Update*` requests with `update.Prepare`, through the shared
[`updatemask`](../updatemask/README.md) normalizer, and runs `FieldMask.Filter` on AIP-157
`Get*` / `List*` / `Search*` / `BatchGet*` responses, all driven by the gRPC method name. Stateless behavior violations (such as REQUIRED cleared) are surfaced as the canonical `InvalidArgument` + `io.altessa.badrequest.v1.BadRequest` shape (the detail type the
protovalidate interceptor also emits) rather than leaking to the storage layer.

By default, a request mask is a singular `google.protobuf.FieldMask` declared as `update_mask` / `read_mask`, falling back to `updateMask` /
`readMask` only when the corresponding snake_case name is not declared. A non-empty `WithMaskFieldName` selects an exact declared name without fallback.

Updates use `update.Prepare` to validate raw paths, normalize the resource, write back the initial effective mask, and retain a deferred plan.
`IMMUTABLE` and `IDENTIFIER` values are captured in the deferred plan for comparison with stored state; their paths do not authorize writes in the initial mask.
Protected paths, including explicit clears, are deferred. An unchanged protected value is ignored; a change is `INVALID_ARGUMENT` from stateful
planning in the handler. `OUTPUT_ONLY` input is sanitized during normalization, including explicit empty and dropped-only masks.
An omitted mask implies populated fields; an explicit empty mask is a no-op. An empty interim mask can still require protected-field comparison.
With explicit paths or an explicit empty mask, the handler-visible resource retains selected input and the resource routing identity.
Unselected containers are dropped even when they contain IMMUTABLE or nested IDENTIFIER fields.
Selected collection elements retain their identifiers, map keys, and list positions; OUTPUT_ONLY input is cleared.
The plan captures original values for deferred comparison; selected protected values remain visible. Omitted masks and * retain protected input.
An omitted mask selects populated leaves; an explicit empty mask clears unselected mutable input and performs no writes or secret changes.
A dropped-only mask also filters unselected input. Writable masks retain selected values for downstream validation; `*` processes replacement selection.
`OUTPUT_ONLY` is cleared in every case. The deferred plan captures original input for comparison, replacements and selected `INPUT_ONLY` secrets.
No `KeepIdentity` cleanup runs before completion. Never persist the normalized request as a replacement.

The handler must call `update.Patch.Run` with the received context and stored-resource callbacks, or call `Spec.Plan` with
`update.WithPlanContext(ctx)` and `update.WithCurrent(stored)`, then merge the final paths. Merging only the written-back mask does not perform
the protected-field check. Always complete the plan, even when the written-back mask is empty.
Map handler errors with `errstatus.WithCanonicalRules()` (or equivalent conversion) to expose protected-field failures as `InvalidArgument`.

Unary handlers receive the plan through `update.ContextWithPlan`. Streaming handlers obtain a context snapshot from `stream.Context()` after
each successful receive. Each message gets its own plan; use a fresh resource instance when retaining earlier contexts. Both interceptors share
a per-message tracker: the first successful normalization and its original mask are retained, and the second interceptor reuses that plan.

## Method classification

The short method name (the part after the final `/`) is matched case-sensitively against AIP prefixes:

| Prefix                                  | Kind         | Effect                                                             |
|-----------------------------------------|--------------|--------------------------------------------------------------------|
| `Update*`, `Patch*`, `BatchUpdate*`     | `KindUpdate` | `PreCall`: `update.Prepare(req)`, writeback the effective mask   |
| `Get*`, `List*`, `Search*`, `BatchGet*` | `KindRead`   | `PreCall` captures `read_mask`; `PostCall` runs `Filter(response)` |
| anything else                           | `KindNone`   | request and response untouched                                     |

`Create*` is deliberately not handled — AIP-133 does not define a mask on the create path. Use `WithMethodKind` to override the classification for a
specific fully-qualified method.

## Options

| Option                                  | Default                         | Description                                                                                                                                                                                 |
|-----------------------------------------|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `WithMethodKind(fullMethod, kind)`      | --                              | Override classification for `info.FullMethod`. Accumulates across calls.                                                                                                                    |
| `WithSkipReadMask()`                    | off                             | Disable the response-side `Filter` pass on `KindRead` methods.                                                                                                                              |
| `WithMaskFieldName(name)`               | declared names above           | Select an exact declared mask field name without fallback. Empty uses the defaults above.                                                                                                 |
| `WithResourceFieldName(name)`           | exactly one non-mask message    | Override the resource carrier field name on Update requests.                                                                                                                                |
| `WithUpdateExtractor(fullMethod, fn)`   | --                              | Per-method update extractor. See [Custom extractors](#custom-extractors).                                                                                                                   |
| `WithReadExtractor(fullMethod, fn)`     | --                              | Per-method read extractor.                                                                                                                                                                  |
| `WithDefaultUpdateExtractor(fn)`        | built-in reflection             | Global default update extractor used when no per-method override matches.                                                                                                                   |
| `WithDefaultReadExtractor(fn)`          | built-in chain                  | Global default read extractor.                                                                                                                                                              |
| `WithMetadataReadMaskHeader(name)`      | `"x-goog-fieldmask"`            | Override the gRPC metadata key the built-in AIP-157 read extractor reads.                                                                                                                   |
| `WithStrictMode()`                      | off                             | Reject missing/empty masks and invalid paths before preparing; emits `google.rpc.BadRequest`.                                                               |
| `WithUpdateOptions(...update.Option)`   | go-aip defaults                 | Options of `update.Prepare`: resolver, identity, immutable and implied-skip fields, atomic messages, path validation, limits.                                                              |
| `WithIgnoreMethods(...string)`          | --                              | Fully-qualified method names to bypass entirely.                                                                                                                                            |
| `WithIgnorePatterns(...*regexp.Regexp)` | `defaults.IgnorePatterns`       | Regex patterns to bypass (default skips reflection and health probes).                                                                                                                      |
| `WithLogger(*slog.Logger)`              | discard                         | Logger for debug/error messages.                                                                                                                                                            |

## Strict update masks

`WithStrictMode()` rejects missing or empty masks and validates every raw path regardless of path-validation mode.
Protected values still require handler completion; OUTPUT_ONLY paths are ignored. Strict refusals carry `google.rpc.BadRequest` details.

The built-in extractor handles the canonical request shape automatically: one top-level `google.protobuf.FieldMask update_mask` and exactly one other
singular message field for the resource. Use `WithUpdateExtractor` for exceptions such as a mask nested in an options message.

A strict `*` mask follows AIP-134's full-replacement convention and excludes `OUTPUT_ONLY` fields. `IMMUTABLE` and `IDENTIFIER` values are compared
with stored state when the handler completes the deferred plan: unchanged populated values are accepted, while changes, including wildcard clears of
non-empty protected values, are rejected.

```go
fieldmask.ServerInterceptor(fieldmask.WithStrictMode())
```

## Custom extractors

The built-in extractor assumes the canonical AIP-134 shape — `UpdateXxxRequest{Xxx resource, FieldMask update_mask}` — and finds the mask on the top
level of the request via descriptor reflection. Requests that nest the mask inside an `Options` sub-message (`req.Options.UpdateMask`) are not reachable
from there.

For those, plug a typed extractor in. The function types and the generic builders live in go-aip
[`fieldmask/extract`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask/extract) so non-gRPC callers can reuse them.

```go
chain := interceptors.NewChain(
    // ... metadata, auth, fieldbehavior, ...
    fieldmask.ServerInterceptor(
        fieldmask.WithMethodKind("/x.v1.X/UpdateBucket", fieldmask.KindUpdate),
        fieldmask.WithUpdateExtractor("/x.v1.X/UpdateBucket",
            extract.NewUpdateFunc(
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
   - Update path: `updatemask.DefaultExtractor` (extends go-aip lookup to omitted masks). Honors `WithMaskFieldName` / `WithResourceFieldName`.
   - Read path: `extract.ChainRead(MetadataReadExtractor, extract.DefaultRead)` — modern AIP-157 metadata header first, deprecated
     AIP-161 request-message `read_mask` second.

`UpdateNormalizer(opts...)` returns the `updatemask.Normalizer` of these options: this extractor resolution (the method is read from the call
metadata), `WithUpdateOptions`, `WithStrictMode`. Give it to the [`fieldbehavior`](../fieldbehavior/README.md)
interceptor (`fieldbehavior.WithUpdateNormalizer`) with the options of this interceptor so both apply an `update_mask` the same way; the gRPC server
factory does this from the `fieldMask` configuration and `UseFieldMaskOptions`, even when this interceptor is disabled.

A custom extractor returning `ok=false` means "I do not handle this request" — the interceptor logs at Debug and passes through. It does **not** fall
back to a lower tier. This keeps the precedence deterministic when callers register an extractor specifically to disable handling for a method.

### AIP-157 metadata read mask

AIP-161 marks `read_mask` on the request message as **deprecated** and forwards callers to AIP-157, which transports the mask through a side channel:
the gRPC metadata key `x-goog-fieldmask` (and the corresponding HTTP `$fields` query parameter, mapped by grpc-gateway). The built-in read chain
consults that header first and falls back to the request-message `read_mask`. An empty or explicit `"*"` header is treated like a missing one (both mean
"all fields" per AIP-157), so the `read_mask` still applies; the response passes through unfiltered only when neither source supplies a restrictive
mask.

Override the header name with `WithMetadataReadMaskHeader("x-custom-fieldmask")`. The override applies only to the built-in chain; callers that replace
the read extractor wholesale via `WithDefaultReadExtractor` or `WithReadExtractor` own their own header convention and can reuse the metadata extractor:

| Symbol                                                   | Purpose                                                                                        |
|----------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `MetadataReadExtractor(opts ...MetadataExtractorOption)` | Reads the field mask from incoming gRPC metadata per AIP-157.                                  |
| `WithMetadataHeader(name)`                               | Override the metadata key. Defaults to `DefaultMetadataReadMaskHeader` (`"x-goog-fieldmask"`). |
| `DefaultMetadataReadMaskHeader`                          | Constant for the default header name.                                                          |

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
read := extract.ChainRead(
    fieldmask.MetadataReadExtractor(fieldmask.WithMetadataHeader("x-custom-fieldmask")), // AIP-157 modern path
    extract.DefaultRead(),                                                                // AIP-161 deprecated request-field fallback
)
```

### Writeback

`extract.UpdateFunc` returns a writeback closure. The interceptor calls it with the effective mask `update.Prepare` returns — for the nested
example above, that updates `req.Options.UpdateMask` so the handler sees a coherent `(mask, resource)` pair. The writeback is required: the mask as
sent may cover server-managed fields the resource no longer holds, so an extractor that locates a mask but returns no writeback fails the request
with `Internal`.

### Pushing the read mask down to storage

`ReadMaskFromContext(ctx)` hands the handler the read mask the interceptor captured for the current `KindRead` call — resolved by the same extractor
chain, returned as a copy — so the selection can become a database projection through go-aip [`projection`](https://github.com/altessa-s/go-aip/tree/develop/projection) instead of
loading the full resource and trimming it afterwards. It reports `ok=false` for non-read methods, skipped calls and requests without a mask (AIP-157:
all fields). On a server stream, call it after the first `Recv`.

```go
if mask, ok := fieldmask.ReadMaskFromContext(ctx); ok {
    spec, err := parser.ParsePaths(ctx, mask.GetPaths())
    if err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    proj, err = translator.Translate(spec)
}
```

The response filter still runs and is a no-op on a response storage already trimmed; `WithSkipReadMask()` turns it off while the mask stays available
through `ReadMaskFromContext`. Mask paths address the response message the filter runs on: for a `Get*` that is the resource itself, for a `List*` the
envelope (`items.name, next_page_token`). Re-root an envelope mask at the collection field with `Spec.Relative("items")` before translating it;
`ok=false` means the items were not requested at all.

## Usage

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
srvOpts, err := chain.ServerOptions()
if err != nil {
    return err
}
server := grpc.NewServer(srvOpts...)
```

`fieldmask` declares dependencies on `metadata` and `auth` — both must run earlier in the chain. Place `fieldbehavior` (and any payload sanitization)
before `fieldmask` so the mask sees the already-stripped request, and place `protovalidator` after `fieldmask` so the validator runs against the final
mask-cleaned payload.

### From configuration

The gRPC server factory wires the interceptor from the `interceptors.fieldMask` section of [`config/grpc`](../../../../config/grpc/README.md)
(`enabled`, `ignoreMethods`, `ignorePatterns`, `skipReadMask`, `maskFieldName`, `resourceFieldName`,
`metadataReadMaskHeader`; disabled by default) between `fieldbehavior` and `protovalidator`. Method kinds and extractors are code-level and are added
with `UseFieldMaskOptions`:

```go
srv, err := factory.New(cfg.Grpc).
    UseFieldMaskOptions(fieldmask.WithMethodKind("/x.v1.X/Archive", fieldmask.KindUpdate)).
    WithInterceptors().
    Build()
```

## Errors

| Source                                           | gRPC status       | Detail attached                       |
|--------------------------------------------------|-------------------|---------------------------------------|
| `*update.BehaviorError`                          | `InvalidArgument` | `io.altessa.badrequest.v1.BadRequest` |
| `*fieldmask.ValidationError` (bad mask path)     | `InvalidArgument` | --                                    |
| `*update.StrictError` (strict mode)              | `InvalidArgument` | `google.rpc.BadRequest`               |
| `read_mask` path the response does not hold      | `InvalidArgument` | --                                    |
| Reflection miss (no `update_mask` / `read_mask`) | --                | silent passthrough, logged at Debug   |
| Update extractor with no writeback               | `Internal`        | logged at Error, request rejected     |
| Panic inside an extractor, `Prepare`, `Filter` | `Internal`        | logged at Error, request rejected     |

Each behavior violation carries the dot-separated field path (`field_path`) and a human description (`message`: `"immutable field cannot be
modified"`, `"identifier field cannot be modified"`, `"required field cannot be cleared"`). The go-atlas gRPC client maps it into `client.Error.Fields`.

## Streaming

`PostMsgReceive` and `PreMsgSend` mirror the unary path so the same per-method classification applies to every message in a streaming RPC; the read mask
is applied to a clone of each outgoing message before it reaches the transport. Classification is computed once at stream start from `info.FullMethod`;
the `read_mask` of the first request is captured once, as an owned copy, and reused for every response frame: later requests and handler changes to the
request do not alter it, and concurrent sends and receives on a bidi stream are safe.

## See also

- go-aip [`fieldmask`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask), [`fieldmask/update`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask/update) and [`fieldmask/extract`](https://github.com/altessa-s/go-aip/tree/develop/fieldmask/extract) — the
  underlying `FieldMask` operations, the update check and the `UpdateMask` / `ReadMask` / `SetUpdateMask` reflection helpers this interceptor uses.
- [go-aip field mask guide](https://github.com/altessa-s/go-aip/tree/develop/docs/fieldmask.md) — full reference, traversal semantics, performance.
- [`transport/grpc/interceptors/fieldbehavior`](../fieldbehavior/README.md) — sibling AIP-203 interceptor; pair before `fieldmask`.
- [`transport/grpc/interceptors/protovalidator`](../protovalidator/README.md) — pair after `fieldmask` so validation runs on the mask-cleaned payload.

## Policy propagation

`WithUpdateOptions` supplies the `update.Option` policy of `update.Prepare`; the normalizer appends `update.WithUpdateMaskFieldName` with the
configured mask field name and `update.WithStrict` last. `WithMaskFieldName` also controls the field name in strict
validation error details.

## Diagnostics

An invalid read mask returns `InvalidArgument` with `ErrorInfo.Reason` set to `INVALID_FIELD_MASK`. The rejected path is omitted from the status and diagnostics.
