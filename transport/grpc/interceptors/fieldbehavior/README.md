# fieldbehavior

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
```

Server-side gRPC interceptor that runs the matching
[`domain/proto/fieldbehavior`](../../../../domain/proto/fieldbehavior/README.md) `Strip*` function against the request and the response based on the
method name. Operates in mutation mode — it clears server-managed and INPUT_ONLY fields in place. Strict mode is intentionally not exposed here:
rejecting payloads at the interceptor layer is a contract decision that belongs in the handler.

## Method classification

The short method name (the part after the final `/`) is matched case-sensitively against AIP-133/AIP-134 prefixes:

| Prefix                       | Kind          | Effect                                          |
|------------------------------|---------------|-------------------------------------------------|
| `Create*`, `BatchCreate*`    | `KindCreate`  | `StripCreate(req)` runs in `PreCall`            |
| `Update*`, `Patch*`, `BatchUpdate*` | `KindUpdate` | `StripUpdate(req)` runs in `PreCall`     |
| anything else                | `KindNone`    | request untouched, response still stripped     |

Successful responses always pass through `StripResponse` (INPUT_ONLY → cleared) unless [`WithSkipResponse`](#options) is set or the method is
classified `KindSkip`. The handler-error path skips response stripping because gRPC drops the body anyway.

## Options

| Option                                | Default                | Description                                                                       |
|---------------------------------------|------------------------|-----------------------------------------------------------------------------------|
| `WithMethodKind(fullMethod, kind)`    | --                     | Override classification for `info.FullMethod`. Accumulates across calls.          |
| `WithSkipResponse()`                  | off                    | Disable the automatic response strip (e.g. if the handler already does it).       |
| `WithMaxStripDepth(n)`                | `DefaultMaxStripDepth` (32) | Cap on descriptor traversal depth passed to every Strip* call.               |
| `WithIgnoreMethods(...string)`        | --                     | Fully-qualified method names to bypass entirely.                                  |
| `WithIgnorePatterns(...*regexp.Regexp)` | `defaults.IgnorePatterns` | Regex patterns to bypass (default skips reflection and health probes).      |
| `WithLogger(*slog.Logger)`            | discard                | Logger for debug/error messages.                                                  |

## Usage

```go
import (
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
)

server := grpc.NewServer(grpc.UnaryInterceptor(
    interceptors.Chain(
        // ... metadata, auth, ...
        fieldbehavior.ServerInterceptor(
            fieldbehavior.WithMethodKind("/x.v1.X/ImportResource", fieldbehavior.KindCreate),
            fieldbehavior.WithMethodKind("/x.v1.X/RotateKey",      fieldbehavior.KindSkip),
        ),
        // ... protovalidator, handler ...
    ),
))
```

`fieldbehavior` declares dependencies on `metadata` and `auth` — both must run earlier in the chain. Place `protovalidator` after
`fieldbehavior` so the validator never sees client-supplied `OUTPUT_ONLY` fields the strip is about to clear.

## Streaming

The interceptor implements `PostMsgReceive` / `PostMsgSent` so the same per-method strip applies to every message in a streaming RPC. Method
classification is computed once at stream start from `info.FullMethod` and reused for every frame.

## Errors

Strip failures are converted to `codes.Internal`. The only reachable strip failure is `fieldbehavior.ErrMaxDepthExceeded`; it is wrapped so
`errors.Is(err, fieldbehavior.ErrMaxDepthExceeded)` continues to work on the gRPC status error.

Panics inside the strip are recovered, logged, and surfaced as `codes.Internal` — the request is rejected rather than allowed through with a
half-mutated payload.

## See also

- [`domain/proto/fieldbehavior`](../../../../domain/proto/fieldbehavior/README.md) — the underlying Strip* functions and option set.
- [`docs/domain/proto/fieldbehavior.md`](../../../../docs/domain/proto/fieldbehavior.md) — full reference, traversal semantics, performance.
- [`transport/grpc/interceptors/protovalidator`](../protovalidator/README.md) — pair with this interceptor; run validation **after** stripping.
