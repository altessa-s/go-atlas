# Field Behavior (`fieldbehavior`)

```go
import "github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"
```

> The package lives under `domain/proto/`. It is documented here alongside the other guides because in practice it sits on the request/response data path
> and is most useful next to [`filter`](../../data/filter.md), [`orderby`](../../data/orderby.md), and the rest of the data toolkit.

A `proto.Message`-stripper driven by Google's `google.api.field_behavior` annotations (AIP-203). The library walks the message payload itself and clears
fields that should not be present in the current direction of the wire — `OUTPUT_ONLY` / `IDENTIFIER` on Create, those plus `IMMUTABLE` on Update, and
`INPUT_ONLY` on responses. The companion package [`fieldmask`](../../../domain/proto/fieldmask/README.md) deals with the `FieldMask` *value*; `fieldbehavior`
deals with the resource *payload*.

---

## When to reach for `fieldbehavior`

| Scenario                                                                                 | Use                                                |
|------------------------------------------------------------------------------------------|----------------------------------------------------|
| Strip server-managed identifiers/timestamps from a Create-request resource              | `StripCreate(req.GetResource())`                   |
| Same for Update, plus reject changes to `IMMUTABLE` fields                              | `StripUpdate(req.GetResource())`                   |
| Strip secrets (`INPUT_ONLY` — passwords, tokens) from a response before returning it    | `StripResponse(resp)`                              |
| Surface client misuse instead of silently clearing populated fields                     | Add `WithStrict()` → `*BehaviorViolationError`     |
| Cap traversal depth on adversarial/fuzzed input                                         | `WithMaxDepth(n)`                                  |
| Strip a custom set of behaviors (e.g. `REQUIRED` for a dry-run validator)               | `Strip(msg, WithBehaviors(...))`                   |
| Wire up the same logic across every gRPC handler                                        | Server interceptor that dispatches by method name  |

---

## Quick start

### Create — clear server-managed fields

```go
func (s *server) CreateBucket(ctx context.Context, req *pb.CreateBucketRequest) (*pb.Bucket, error) {
    if err := fieldbehavior.StripCreate(req.GetBucket()); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    // req.Bucket.Id, req.Bucket.CreateTime, req.Bucket.UpdateTime are now zero —
    // the server is free to mint them.
    return s.repo.Create(ctx, req.GetBucket())
}
```

`DefaultCreateBehaviors = [OUTPUT_ONLY, IDENTIFIER]`. `REQUIRED`, `IMMUTABLE` and `INPUT_ONLY` survive — the client is allowed (and often expected) to
supply them at create time.

### Update — additionally lock immutable fields

```go
func (s *server) UpdateBucket(ctx context.Context, req *pb.UpdateBucketRequest) (*pb.Bucket, error) {
    if err := fieldbehavior.StripUpdate(req.GetBucket()); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    return s.repo.Update(ctx, req.GetBucket(), req.GetOptions().GetUpdateMask())
}
```

`DefaultUpdateBehaviors = [OUTPUT_ONLY, IDENTIFIER, IMMUTABLE]`. Combine with [`fieldmask.ApplyUpdateMask`](../../../domain/proto/fieldmask/README.md) for
full AIP-134 update semantics — `fieldbehavior` clears the payload, `fieldmask` validates the mask.

### Response — strip INPUT_ONLY secrets

```go
func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
    u, err := s.repo.Get(ctx, req.GetId())
    if err != nil { return nil, err }
    if err := fieldbehavior.StripResponse(u); err != nil {
        return nil, status.Error(codes.Internal, err.Error())
    }
    return u, nil
}
```

`DefaultResponseBehaviors = [INPUT_ONLY]`. Use on every read-side return value where a field marked `INPUT_ONLY` (a password, a one-shot token, an API
secret) might have been loaded from storage. The strip is cheap (~16 µs on a fully populated `Bucket`-sized message — see [Performance](#performance)).

### Custom behavior set

```go
// Validate that REQUIRED fields are populated by stripping them under strict
// mode and inspecting the violation list.
err := fieldbehavior.Strip(msg,
    fieldbehavior.WithBehaviors(annotations.FieldBehavior_REQUIRED),
    fieldbehavior.WithStrict(),
)
var vErr *fieldbehavior.BehaviorViolationError
if errors.As(err, &vErr) {
    for _, v := range vErr.Violations {
        // v.Path = "user.email", v.Behavior = REQUIRED
    }
}
```

---

## API surface

| Entry point                                  | Default behavior set                                | Description                                                    |
|----------------------------------------------|-----------------------------------------------------|----------------------------------------------------------------|
| `Strip(msg, opts...)`                        | none (no-op without `WithBehaviors`)                | Generic walk; configure with `WithBehaviors`.                  |
| `StripCreate(msg, opts...)`                  | `DefaultCreateBehaviors`                            | OUTPUT_ONLY + IDENTIFIER.                                      |
| `StripUpdate(msg, opts...)`                  | `DefaultUpdateBehaviors`                            | OUTPUT_ONLY + IDENTIFIER + IMMUTABLE.                          |
| `StripResponse(msg, opts...)`                | `DefaultResponseBehaviors`                          | INPUT_ONLY.                                                    |

All four return `error`. The error is `nil` in mutation mode unless `MaxDepth` is exceeded; in strict mode it is `*BehaviorViolationError` (see
[Strict mode](#strict-mode)) when at least one populated field matched the configured set.

### Options

| Option                                       | Default          | Description                                                                                                                       |
|----------------------------------------------|------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `WithBehaviors(set ...annotations.FieldBehavior)` | —              | Replace the configured behavior set entirely. Passing it to `StripCreate`/`Update`/`Response` overrides their defaults.            |
| `WithStrict()`                               | off              | Do not mutate the message; instead collect every populated stripped field into `*BehaviorViolationError` and return it.            |
| `WithMaxDepth(n int)`                        | 32               | Cap on traversal depth. Protocol Buffers schemas cannot contain cycles, so hitting the limit indicates adversarial / fuzzed input. |

### Defaults

| Constant / variable                          | Value                                                                                                                                                |
|----------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `DefaultStrict`                              | `false`                                                                                                                                              |
| `DefaultMaxDepth`                            | `32`                                                                                                                                                 |
| `DefaultCreateBehaviors`                     | `[OUTPUT_ONLY, IDENTIFIER]`                                                                                                                          |
| `DefaultUpdateBehaviors`                     | `[OUTPUT_ONLY, IDENTIFIER, IMMUTABLE]`                                                                                                               |
| `DefaultResponseBehaviors`                   | `[INPUT_ONLY]`                                                                                                                                       |

### Errors

| Symbol                                       | Description                                                                                                              |
|----------------------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| `*BehaviorViolationError`                    | Aggregate of every populated stripped field under `WithStrict`. Carries a `Violations []BehaviorViolation` slice.        |
| `BehaviorViolation{ Path, Behavior }`        | One entry per field. `Path` is a dot/index path (`aliases[2].id`, `labels["k"].id`); `Behavior` is the first match.      |
| `ErrMaxDepthExceeded`                        | `sentinel error` returned when traversal exceeds `WithMaxDepth`. Use `errors.Is` to detect.                              |

---

## Traversal semantics

The walker visits every populated field of the root message in descriptor order, applies the match rule, and either clears the field (mutation mode) or
records a violation (strict mode) — see [Strict mode](#strict-mode).

| Field shape                                          | If the field itself is annotated     | If the field has no annotation                          |
|------------------------------------------------------|--------------------------------------|---------------------------------------------------------|
| Scalar (`string`, `int32`, `bool`, …)               | `FieldDescriptor.Clear`              | No-op.                                                  |
| Singular nested message (`Profile profile = 7`)     | Subtree cleared, **no recursion**.   | Recursive descent into the populated message.           |
| Repeated of messages (`repeated Profile aliases`)   | Whole list cleared.                  | Recurse into each element; path `field[i]`.             |
| Repeated of scalars (`repeated string tags`)        | List cleared.                        | No-op.                                                  |
| Map<K, Message> (`map<string, Profile> labels`)     | Whole map cleared.                   | Recurse into each value; path `field["key"]`.           |
| Map<K, V> with scalar V                              | Map cleared.                         | No-op.                                                  |
| Oneof case (`oneof source { … }`)                    | Active case cleared if it matches.   | If the active case is a message, recurse into it.       |
| Well-known type (`Timestamp`, `Duration`, …)         | Treated as a leaf; whole field clear | No-op (we do **not** descend into well-known internals).|

Path construction:

* Dot-separated for singular nested messages: `profile.id`, `audit.display_name`.
* Square brackets with integer index for repeated elements: `aliases[0].secret`.
* Square brackets with quoted key for map entries: `labels["primary"].id`. Map iteration order matches `protoreflect.Map.Range`, which is randomised
  — the resulting violations list is **not** stable across runs. Sort it on the caller side if you need determinism.

### Multiple behaviors on the same field

Fields can carry several behaviors at once (`REQUIRED + IMMUTABLE` is the canonical example for a slug). The walker matches *any* of the configured set
— so `StripUpdate` clears such a field because `IMMUTABLE` intersects `DefaultUpdateBehaviors`, while `StripCreate` leaves it alone because neither
`REQUIRED` nor `IMMUTABLE` is in `DefaultCreateBehaviors`.

When recording a violation under `WithStrict`, the reported `Behavior` is the **first** match against the configured set — useful for tagging the
violation but not authoritative; call `Get(fd)` from `domain/proto/internal/behavior` (unexported) or read the descriptor yourself if you need every value.

### What is **not** traversed

* Subtrees of a tagged parent field. Once a field matches and is cleared, descendants are gone with it — there is no need to descend.
* Well-known types. `Timestamp.seconds` is technically a scalar, but a `create_time` field annotated `OUTPUT_ONLY` clears the whole `Timestamp` value, and
  inspecting `seconds`/`nanos` individually never makes sense in this context.
* Unpopulated fields (`prf.Has(fd) == false`). The walker skips them entirely — even in strict mode, an absent field can never be a violation.

---

## Strict mode

`WithStrict()` flips the contract:

* **Mutation mode (default):** the walk mutates `msg` in place, returns `nil` (or `ErrMaxDepthExceeded`). Fast — single pass over set fields.
* **Strict mode (`WithStrict`):** the walk does **not** mutate `msg`. Every populated field whose behavior matches the set is appended to a
  `[]BehaviorViolation`. If the slice is non-empty, `Strip` returns `*BehaviorViolationError` wrapping it; `msg` is left untouched.

Use strict mode when you want the server to **complain** instead of silently fixing up. Two typical patterns:

```go
// Reject Create requests that send fields the server owns.
if err := fieldbehavior.StripCreate(req.GetBucket(), fieldbehavior.WithStrict()); err != nil {
    var vErr *fieldbehavior.BehaviorViolationError
    if errors.As(err, &vErr) {
        return nil, status.Errorf(codes.InvalidArgument,
            "request sets server-managed fields: %v", vErr.Violations)
    }
    return nil, status.Error(codes.Internal, err.Error())
}
// Else: nothing to strip, payload is clean.
```

```go
// Audit response for INPUT_ONLY leakage (alarm, do not block).
if err := fieldbehavior.StripResponse(resp, fieldbehavior.WithStrict()); err != nil {
    var vErr *fieldbehavior.BehaviorViolationError
    if errors.As(err, &vErr) {
        s.logger.Warn("input_only leak", "method", info.FullMethod, "violations", vErr.Violations)
    }
}
// Still call StripResponse without WithStrict afterward to actually clear them.
```

`strict + clear` is two calls — there is no single-pass "report and clear" mode by design. Strict leaves the message untouched so the caller can reject
the request before any business logic runs against partially-cleared data.

---

## Wiring into gRPC

A reusable interceptor pattern (out of scope for the package — wire it up in your gRPC layer):

```go
// stripInterceptor sanitises every Create/Update request body before the
// handler runs, and every response before it goes back to the wire.
func stripInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    switch m := req.(type) {
    case interface{ GetResource() proto.Message }: // hypothetical accessor on request types
        switch {
        case strings.HasPrefix(info.FullMethod, "/x.v1.X/Create"):
            if err := fieldbehavior.StripCreate(m.GetResource()); err != nil {
                return nil, err
            }
        case strings.HasPrefix(info.FullMethod, "/x.v1.X/Update"):
            if err := fieldbehavior.StripUpdate(m.GetResource()); err != nil {
                return nil, err
            }
        }
    }
    resp, err := handler(ctx, req)
    if err != nil || resp == nil {
        return resp, err
    }
    if pm, ok := resp.(proto.Message); ok {
        if err := fieldbehavior.StripResponse(pm); err != nil {
            return nil, err
        }
    }
    return resp, nil
}
```

If you want the method-name → strip-kind mapping to happen automatically — driven by `google.api.method_signature` or the AIP naming conventions —
that lives in your application's interceptor stack, not here.

---

## Performance

Apple M4 Pro, `go test -bench`, `for b.Loop()` driver, full `Resource` fixture with nested messages, lists, maps, oneof, and INPUT_ONLY fields:

```
BenchmarkStripCreate-14            414 µs/op    216 520 B/op    1 401 allocs/op
BenchmarkStripUpdate-14             27 µs/op      5 320 B/op       82 allocs/op
BenchmarkStripResponse-14           16 µs/op      5 496 B/op       90 allocs/op
BenchmarkStripCreateStrict-14       18 µs/op      6 400 B/op      101 allocs/op
BenchmarkStripCreateEmpty-14        44 µs/op        840 B/op       20 allocs/op
```

Notes:

* `StripCreate` is heavier on the first iteration because it clears nested message subtrees, which forces `protoreflect.Mutable` allocations on each
  child. Steady-state numbers (`go test -bench -benchtime=10s`, once the descriptor caches are warm) settle 5–10× lower.
* The empty-resource benchmark isolates the descriptor walk: ~44 µs is the floor for a Resource with ~30 fields and no work to do.
* Strict mode is *faster* than mutation in this fixture because it skips the `protoreflect.Mutable` allocations and only appends to a slice.

If your hot path is `StripResponse` on millions of small messages per second, the dominant cost is the descriptor walk itself; pre-cache the descriptor
field-list (Go ≥ 1.21 protobuf-go already caches it internally), or limit traversal to top-level fields by stripping the inner messages explicitly.

---

## `fieldbehavior` vs `fieldmask`

| Concern                                              | `fieldmask`                                                                 | `fieldbehavior`                                                |
|------------------------------------------------------|-----------------------------------------------------------------------------|----------------------------------------------------------------|
| What is the input?                                   | A `FieldMask` value (paths)                                                 | A `proto.Message` payload                                      |
| What does it produce?                                | A filtered/pruned `proto.Message`, or an error on mask validation           | A mutated `proto.Message` (or `*BehaviorViolationError`)       |
| When does it run on the wire path?                   | Update (validates the `update_mask`)                                        | Create / Update (clears payload) and Response (clears secrets) |
| How does it use `google.api.field_behavior`?         | Validates that the **mask** does not reference IMMUTABLE/OUTPUT_ONLY paths  | Walks the **payload** and clears every matching field          |
| Shared helper                                        | `domain/proto/internal/behavior.Get`                                        | same                                                            |

You almost always want both: `fieldbehavior.StripCreate`/`Update` on the request payload **before** validation, and `fieldmask.ApplyUpdateMask` on the
`update_mask` **after** validation but before writing.

---

## Edge cases & FAQ

**Q: Can I add new behaviors when Google extends the proto?**
A: Yes — `WithBehaviors` takes any `annotations.FieldBehavior` value. Build a slice that includes the new variant; the walker only checks `==` against
the configured set.

**Q: My `oneof` has both an INPUT_ONLY case and a non-secret case. Does `StripResponse` collapse the oneof?**
A: Only if the **active** case is the annotated one. Other cases are not touched (they are not "populated" from `protoreflect`'s perspective). If the
INPUT_ONLY case is active, the entire oneof is cleared — no fallback to a sibling case.

**Q: What about `UNORDERED_LIST` and `NON_EMPTY_DEFAULT`?**
A: They are not part of any default set. `UNORDERED_LIST` is a semantic hint, not an access restriction; `NON_EMPTY_DEFAULT` controls server-side
defaults. You can still feed them to `WithBehaviors` if you have a custom use case.

**Q: What happens to a message that has zero `field_behavior` annotations anywhere?**
A: `Strip*` walks the descriptor, finds no matches, and returns `nil`. The cost is one `protoreflect.Message.Range`-equivalent traversal — see the
`BenchmarkStripCreateEmpty` number above for the order of magnitude.

**Q: Is `Strip` thread-safe?**
A: Read-only on its own state (the options are captured into a stripper that lives for the duration of one call). The input `proto.Message` must **not**
be shared between concurrent `Strip` calls in mutation mode — protoreflect mutations are not safe under concurrency. `WithStrict` is safe to call
concurrently on the **same** message as long as no other goroutine mutates it.

**Q: How do I integrate with `protoc-gen-validate` / `buf validate`?**
A: Order matters. Run `fieldbehavior.StripCreate` / `StripUpdate` **first**, then validation. Otherwise the validator may flag client-supplied
`OUTPUT_ONLY` fields you were about to clear anyway, polluting the error response.

---

## See also

* [`domain/proto/fieldmask`](../../../domain/proto/fieldmask/README.md) — hierarchical field mask: filter, prune, set operations, update-mask validation.
* [`domain/proto/internal/behavior`](../../../domain/proto/internal/behavior) — the shared `field_behavior` annotation reader used by both packages.
* **AIP-203** — `field_behavior` definitions ([aip.dev/203](https://google.aip.dev/203)).
* **AIP-133 / AIP-134** — Create / Update method conventions that drive the `Strip*` default sets.
* **AIP-132** — `order_by` DSL, paired with the `data/orderby` package documented in [`orderby.md`](../../data/orderby.md).
