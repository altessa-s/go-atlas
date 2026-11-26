# fieldbehavior

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
```

Server-side gRPC interceptor that runs the matching [go-aip `fieldbehavior`](https://github.com/altessa-s/go-aip/tree/develop/fieldbehavior)
`Strip*` function against the request and the response based on the method name. Operates in mutation mode — it clears server-managed and
`INPUT_ONLY` fields in place. Strict mode is intentionally not exposed here: rejecting payloads at the interceptor layer is a contract decision
that belongs in the handler.

By default, the Update mask is a singular `google.protobuf.FieldMask` declared as `update_mask`, falling back to `updateMask` only when
`update_mask` is not declared. `WithUpdateNormalizer` can override this lookup through the supplied normalizer.

## Method classification

The short method name (the part after the final `/`) is matched case-sensitively against AIP-133/AIP-134 prefixes:

| Prefix                              | Kind         | Effect                                            |
|-------------------------------------|--------------|---------------------------------------------------|
| `Create*`, `BatchCreate*`           | `KindCreate` | `StripCreate(req)` runs in `PreCall`              |
| `Update*`, `Patch*`, `BatchUpdate*` | `KindUpdate` | `PreCall` applies the `update_mask` (see below)    |
| anything else                       | `KindNone`   | request untouched, response still stripped        |

Following AIP-203, `IDENTIFIER` fields are cleared on Create (the server assigns them) and preserved on Update, where they address the resource being
updated — also when the resource is nested in an `Update*Request` wrapper. `OUTPUT_ONLY` is cleared on both.

Updates use `update.Prepare` to validate raw paths, normalize the resource, write back the initial effective mask, and retain a deferred plan.
`IMMUTABLE` and `IDENTIFIER` values are captured in the deferred plan for comparison with stored state; their paths do not authorize writes in the
initial mask.
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

Fields outside the resource keep their existing traversal: OUTPUT_ONLY values are cleared, ordinary control fields remain, and formats are checked.
IMMUTABLE and IDENTIFIER values outside the resource are retained; handlers must validate these separately against the relevant stored state.
When lookup fails (for example, a bare resource or an unconfigured nested mask), `StripUpdate` sanitizes the whole request and preserves protected
values. That fallback has no deferred mask plan; custom masked requests should configure an extractor so explicit clear intent is retained.
AIP-202 formats are still checked on all retained values, including those of an explicit empty-mask request.

Successful responses always pass through `StripResponse` (`INPUT_ONLY` cleared) unless [`WithSkipResponse`](#options) is set or the method is
classified `KindSkip`. The handler-error path skips response stripping because gRPC drops the body anyway.

## Options

| Option                                  | Default                     | Description                                                                                                                                                                                     |
|-----------------------------------------|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `WithMethodKind(fullMethod, kind)`      | --                          | Override classification for `info.FullMethod`. Accumulates across calls.                                                                                                                        |
| `WithSkipResponse()`                    | off                         | Disable the automatic response strip (e.g. if the handler already does it).                                                                                                                     |
| `WithMaxStripDepth(n)`                  | `DefaultMaxStripDepth` (32) | Cap on request strip traversal and on the full protobuf response tree. |
| `WithIgnoreMethods(...string)`          | --                          | Fully-qualified method names to bypass entirely.                                                                                                                                                |
| `WithIgnorePatterns(...*regexp.Regexp)` | `defaults.IgnorePatterns`   | Regex patterns to bypass (default skips reflection and health probes).                                                                                                                          |
| `WithUpdateNormalizer(n)`               | `updatemask.New()`          | How an Update request's `update_mask` is located and applied; pass `fieldmask.UpdateNormalizer(...)` with the fieldmask options (the server factory does).                                     |
| `WithLogger(*slog.Logger)`              | discard                     | Logger for debug/error messages.                                                                                                                                                                |

## Usage

```go
import (
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
)

chain := interceptors.NewChain(
    // ... metadata, auth, ...
    fieldbehavior.ServerInterceptor(
        fieldbehavior.WithMethodKind("/x.v1.X/ImportResource", fieldbehavior.KindCreate),
        fieldbehavior.WithMethodKind("/x.v1.X/RotateKey",      fieldbehavior.KindSkip),
    ),
    // ... protovalidator, handler ...
)
srvOpts, err := chain.ServerOptions()
if err != nil {
    return err
}
server := grpc.NewServer(srvOpts...)
```

`fieldbehavior` declares dependencies on `metadata` and `auth` — both must run earlier in the chain. Place `protovalidator` after `fieldbehavior`
so the validator never sees client-supplied `OUTPUT_ONLY` fields the strip is about to clear.

## Streaming

The interceptor strips received messages in `PostMsgReceive` and sanitizes outgoing messages in `PreMsgSend`. Unary and stream responses
are checked against the depth limit before copying. The interceptor forwards the original response when it has no populated `INPUT_ONLY`
fields and strips a copy otherwise. Method classification is computed once at call start and reused for every frame.

## Errors

A rejected request (`fieldbehavior.BehaviorViolationError`: a value that breaks its field format)
fails with `codes.InvalidArgument` and an `io.altessa.badrequest.v1.BadRequest` detail with one field violation per offending field, the detail
the `fieldmask` and `protovalidator` interceptors emit. Any other strip failure is converted to `codes.Internal`; the only reachable one is
`fieldbehavior.ErrMaxDepthExceeded`, wrapped so `errors.Is(err, fieldbehavior.ErrMaxDepthExceeded)` continues to work on the gRPC status error.

Panics inside the strip are recovered, logged, and surfaced as `codes.Internal` — the request is rejected rather than allowed through with a
half-mutated payload.

## See also

- [go-aip `fieldbehavior`](https://github.com/altessa-s/go-aip/tree/develop/fieldbehavior) — the underlying Strip* functions and option set.
- [go-aip field behavior guide](https://github.com/altessa-s/go-aip/tree/develop/docs/fieldbehavior.md) — full reference, traversal semantics, performance.
- [`transport/grpc/interceptors/protovalidator`](../protovalidator/README.md) — pair with this interceptor; run validation **after** stripping.

Payload sanitization failures log a status code or a recovery category.
