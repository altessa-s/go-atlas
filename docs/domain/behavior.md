# Behavior (`behavior`)

```go
import "github.com/altessa-s/go-atlas/domain/behavior"
```

The reflection-based counterpart to [`fieldbehavior`](proto/fieldbehavior.md): where that package reads `google.api.field_behavior` off a
`proto.Message` descriptor, `behavior` reads the same vocabulary from `behavior:"…"` tags on an ordinary Go struct. One struct declares which fields are
server-owned, immutable, or input-only — instead of a separate Create/Update/storage type per operation.

The package is a **core** that runs a single reflection walk over a struct and hands the resolved tree to a pluggable `Translator`. The same walk can feed
different output models: the built-in `Strip`/`Clean` produce a cleaned struct; backend translators under [`translators/`](../../domain/behavior/translators)
fold it into something else (a MongoDB `bson.M`, …). `behavior` itself stays dependency-free.

---

## Two ways to use it

| You want…                                                          | Use                                                                            |
|--------------------------------------------------------------------|-------------------------------------------------------------------------------|
| Clear server-owned fields on a domain struct, in place            | `StripCreate(&v)` / `StripUpdate(&v)` / `StripResponse(&v)`                    |
| The same, but keep the original and return a cleaned copy         | `Clean(v, WithKinds(...))`                                                     |
| Report populated forbidden fields instead of clearing them        | add `WithStrict()` → `*ViolationError`                                         |
| Fold the struct into a different output model                      | `New[Out](translator, opts...).Translate(ctx, &v)`                            |
| Build a MongoDB update/insert document or projection              | the [`translators/mongo`](../../domain/behavior/translators/mongo) translators |

"Strip" clears a field to its zero value, which reads the same for every representation: a plain `T` becomes its zero value, a `*T` becomes `nil`, an
[`optional.Optional`](../../core/types/optional) becomes `None`, and a slice or map becomes `nil`. Callers may model partial updates with either `*T` or
`Optional`; the outcome is identical.

---

## Default output: `Strip*` / `Clean`

```go
type Bucket struct {
    ID         string                    `behavior:"identifier"`
    TenantID   string                    `behavior:"immutable"`
    CreateTime time.Time                 `behavior:"output_only"`
    Password   string                    `behavior:"input_only"`
    Policy     optional.Optional[Policy]
}

// Reject server-owned fields the client must not supply on Create.
if err := behavior.StripCreate(&bucket); err != nil {
    return err
}
```

| Function        | Default kinds                           | Typical use                               |
|-----------------|-----------------------------------------|-------------------------------------------|
| `StripCreate`   | `OutputOnly`, `Identifier`              | Create payloads                           |
| `StripUpdate`   | `OutputOnly`, `Identifier`, `Immutable` | Update payloads                           |
| `StripResponse` | `InputOnly`                             | Server responses (passwords, tokens)      |
| `Strip`         | none (no-op unless `WithKinds`)         | Custom combinations, in place             |
| `Clean`         | none (no-op unless `WithKinds`)         | Custom combinations, returns a deep copy  |

`Strip*` take `*S` and clear in place; `Clean` takes `S` and returns a deep-cleaned copy, leaving the original untouched. A typed-nil pointer is a no-op; a
pointer to a non-struct is an error. Under `WithStrict()` the value is left untouched and a `*ViolationError` lists every populated field that would have
been cleared (each `Violation` carries a `Path` and `Kind`).

### Tag vocabulary

| Tag token     | Kind         | Stripped by                  |
|---------------|--------------|------------------------------|
| `required`    | `Required`   | (validation intent; no default set) |
| `output_only` | `OutputOnly` | `StripCreate`, `StripUpdate` |
| `input_only`  | `InputOnly`  | `StripResponse`              |
| `immutable`   | `Immutable`  | `StripUpdate`                |
| `identifier`  | `Identifier` | `StripCreate`, `StripUpdate` |

Combine with commas (`behavior:"required,immutable"`); a field matches when any of its kinds intersect the strip set. An unrecognized token is an error.

### Options

| Option         | Default      | Description                                                                                       |
|----------------|--------------|---------------------------------------------------------------------------------------------------|
| `WithKinds`    | n/a          | Override the strip set. Passed to a convenience function it replaces (not extends) the default.    |
| `WithStrict`   | off          | Return `*ViolationError` listing populated stripped fields instead of clearing them.               |
| `WithMaxDepth` | `32`         | Cap on nested-struct traversal depth; guards against self-referential graphs.                      |
| `WithTagName`  | `"behavior"` | Read a different struct tag key.                                                                   |
| `WithSchemaWalk` | off        | Type-driven walk for read-only translators (see [Translators](#translators)). Ignored by `Strip`/`Clean`. |
| `WithPooling`  | off          | Engine-only: recycle the resolved tree through an internal pool (see [Translators](#translators)).  |

---

## Translators

For outputs other than a cleaned struct, inject a `Translator` into `New` and call `Translate`. The engine resolves the struct into an `Object`/`Field`
tree once and folds it with the translator, so a single reflection walk feeds any output model.

```go
type Translator[Out any] interface {
    Translate(ctx context.Context, root Object) (Out, error)
}

eng := behavior.New[Out](myTranslator, behavior.WithKinds(behavior.OutputOnly))
out, err := eng.Translate(ctx, &v) // v: struct or non-nil pointer to struct
```

`WithKinds` on the engine decides which fields are marked `Field.Strip`; the translator decides what to do with them (drop, project, …). `Resolve` returns
the same `Object` tree without folding, for translators that recurse into values discovered at run time.

In the resolved tree a nested struct hangs off `Field.Nested`; slice/array/map elements hang off `Field.Collection` as one `Object` per element, with map
keys in `Collection.Keys`. The walk builds that tree on an internal chunked arena. A translator must not retain `root` — or anything reachable from it —
after `Translate` returns, and its output must not alias that memory: under `WithPooling` the engine reuses the arena for later walks, so a retained
reference would silently be overwritten with another value's data. `Strip`/`Clean` always pool internally (their tree never escapes the call); for engines
pooling is opt-in — enable it on hot paths once the translator honors the retention rule.

### Schema-walk

By default the walk is value-driven: nil pointers and empty collections are not descended. A **type-driven** translator (e.g. a projection that must
enumerate nested field paths regardless of values) builds the engine with `WithSchemaWalk()`, which descends nested struct *types* even when the instance
value is a nil pointer or an empty/nil slice/array/map (one representative element per collection). Schema-walk is for read-only translators only —
`Strip`, `Clean`, and the in-place fold never use it.

### MongoDB translators

[`translators/mongo`](../../domain/behavior/translators/mongo) exposes only `behavior.Translator[bson.M]` constructors — the strip-kind and tag policy lives
on the engine via `behavior.WithKinds` / `behavior.WithTagName`, and the package keeps only its own concern, the `bson` tag (`WithBsonTagName`).

```go
import (
    "github.com/altessa-s/go-atlas/domain/behavior"
    mongotr "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

// Update document: {"$set": …, "$unset": …}, _id removed from $set, fields whose
// kinds hit the strip set excluded. Reuse the core's exported kind set.
upd := behavior.New[bson.M](mongotr.NewUpdateTranslator(),
    behavior.WithKinds(behavior.DefaultUpdateKinds...))
doc, err := upd.Translate(ctx, &entity)

// Insert document: every exported field (no kind stripping).
ins := behavior.New[bson.M](mongotr.NewInsertTranslator())
insertDoc, err := ins.Translate(ctx, &entity)

// Exclusion projection: {path: 0} for InputOnly fields, dot-notation for nested.
// Type-driven — requires WithSchemaWalk; pass a zero value as the type carrier.
proj := behavior.New[bson.M](mongotr.NewProjectionTranslator(),
    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
projection, err := proj.Translate(ctx, Entity{})
```

This is independent of the legacy [`data/mongo`](../../data/mongo) converter, which keeps its own CSFLE-aware parser. Reach for the translator when you want a
behavior-tag-driven document or projection without the encryption machinery.

---

## Traversal semantics

| Field shape                                       | Behavior on field   | Behavior absent                     |
|---------------------------------------------------|---------------------|-------------------------------------|
| Scalar; opaque struct (`time.Time`, `Optional`)  | Field cleared       | No-op (leaf)                        |
| Nested struct / `*struct`                         | Whole field cleared | Recursive descent                   |
| Slice/array of structs                            | Field cleared       | Recursive descent into each element |
| Map with struct values                            | Field cleared       | Recursive descent into each value   |
| Nested collections (`[][]T`, `map[K][]T`, …)      | Field cleared       | Recursive descent down to struct elements |

A `behavior` tag placed on a field *inside* an `Optional` is never applied — `Optional` is an opaque leaf, cleared whole only when the field holding it is
itself tagged. Tags on unexported fields are validated (a typo still fails loudly) but never strip, since unexported fields cannot be set. Unlike
`encoding/json`, name shadowing between an outer field and a field promoted from an embedded struct is not resolved: both are processed independently.

---

## See also

* [`domain/behavior` README](../../domain/behavior/README.md) — package reference (functions, options, tag vocabulary).
* [`domain/behavior/translators/mongo`](../../domain/behavior/translators/mongo/README.md) — the MongoDB translator subpackage.
* [`domain/proto/fieldbehavior`](proto/fieldbehavior.md) — the `proto.Message` equivalent driven by `google.api.field_behavior`.
* [`core/types/optional`](../../core/types/optional) — the `Some`/`None` type that round-trips cleanly through a strip.
* **AIP-203** — `field_behavior` definitions ([aip.dev/203](https://google.aip.dev/203)).
