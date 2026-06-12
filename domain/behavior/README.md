# behavior

```go
import "github.com/altessa-s/go-atlas/domain/behavior"
```

Package `behavior` strips fields from a plain Go struct based on `behavior` struct tags — the reflection-based counterpart to
[`domain/proto/fieldbehavior`](../proto/fieldbehavior/README.md). Where the proto package reads `google.api.field_behavior` off a
`proto.Message` descriptor, this package reads an equivalent vocabulary from `behavior:"…"` tags on an ordinary domain model, so one struct
can declare which fields are server-owned, immutable, or input-only — instead of maintaining a separate Create/Update/storage struct per
operation.

"Strip" clears a field by setting it to its zero value, which works the same for every representation: a plain `T` becomes its zero value, a
`*T` becomes `nil`, an [`optional.Optional`](../../core/types/optional) becomes `None`, and a slice or map becomes `nil`. Callers are free to
model partial updates with either `*T` or `Optional`; the outcome is identical.

## Functions

| Function          | Default behaviors                       | Typical use                                |
|-------------------|-----------------------------------------|--------------------------------------------|
| `StripCreate`     | `OutputOnly`, `Identifier`              | Create payloads                            |
| `StripUpdate`     | `OutputOnly`, `Identifier`, `Immutable` | Update payloads                            |
| `StripResponse`   | `InputOnly`                             | Server responses (passwords, auth tokens)  |
| `Strip`           | none (no-op unless `WithKinds`)         | Custom combinations, in place              |
| `Clean`           | none (no-op unless `WithKinds`)         | Custom combinations, returns a copy        |

`Strip*` take `*S` and clear in place; `Clean` takes `S` and returns a deep-cleaned copy, leaving the original untouched.

## Architecture: core + translator

The package is a core that runs a single reflection walk and hands the resolved tree to a pluggable `Translator`, so one walk can feed
different output models. `Strip`/`Clean` are the built-in default outputs (a cleaned struct); other outputs live in their own subpackages
under [`translators/`](translators) and depend only on `behavior` plus their backend driver — `behavior` itself stays dependency-free.

```go
type Translator[Out any] interface {
    Translate(ctx context.Context, root Object) (Out, error)
}

// A translator is injected into New; the same core walk feeds any output model.
eng := behavior.New[[]string](myTranslator, behavior.WithKinds(behavior.OutputOnly))
out, err := eng.Translate(ctx, &bucket)
```

`New` injects the translator; `Engine.Translate` resolves a struct or non-nil pointer-to-struct into an `Object`/`Field` tree (a stripped
field is a leaf; a nested struct hangs off `Field.Nested`; slice/array/map elements — through arbitrarily nested collection layers — hang
off `Field.Collection` as one `Object` per element, with map keys in `Collection.Keys`; an element that is itself a collection resolves to
a synthetic `Object` with a single unnamed `Field`) and folds it with the translator. `Resolve` returns the same `Object` tree without
folding, so a translator can recurse into values it meets at run time.

A translator must not retain `root` — or any `Object`, `Field`, or `Collection` reachable from it — after `Translate` returns, and the
returned value must not alias that memory. Under `WithPooling` the engine recycles the tree's storage for later walks, so a retained
reference would silently be overwritten with another value's data.

## Translators

| Translator                                       | Output  | Purpose                                                          |
|--------------------------------------------------|---------|-----------------------------------------------------------------|
| `Strip` / `Clean` (this package)                 | struct  | Cleaned struct, in place or as a copy — the default output      |
| [`translators/mongo`](translators/mongo)         | `bson.M`| MongoDB `$set`/`$unset` documents and exclusion projections     |

## Tag vocabulary

| Tag token       | Kind          | Stripped by                       |
|-----------------|---------------|-----------------------------------|
| `required`      | `Required`    | (validation intent; no default set) |
| `output_only`   | `OutputOnly`  | `StripCreate`, `StripUpdate`      |
| `input_only`    | `InputOnly`   | `StripResponse`                   |
| `immutable`     | `Immutable`   | `StripUpdate`                     |
| `identifier`    | `Identifier`  | `StripCreate`, `StripUpdate`      |

Combine with commas (`behavior:"required,immutable"`); a field matches when any of its behaviors intersect the strip set. An unrecognized
token is a build-time-style error returned by `Strip`.

## Options

| Option           | Default     | Description                                                                                 |
|------------------|-------------|---------------------------------------------------------------------------------------------|
| `WithKinds`      | n/a         | Override the strip set. Passing it to a convenience function replaces (not extends) the default. |
| `WithStrict`     | off         | Return `*ViolationError` listing populated stripped fields instead of clearing them.        |
| `WithSchemaWalk` | off         | Resolve nested struct types even when the value is absent (type-complete `Object`).         |
| `WithMaxDepth`   | `32`        | Cap on nested-struct traversal depth; guards against self-referential graphs.               |
| `WithTagName`    | `"behavior"`| Read a different struct tag key.                                                            |
| `WithPooling`    | off         | Recycle the engine's resolved tree through an internal pool; the translator must not retain it (see `Translator`). |

### Schema walk

By default the walk stops where a nested value is absent (a nil pointer-to-struct, an empty slice/map of structs). `WithSchemaWalk` makes
it resolve nested struct *types* anyway: a nil pointer descends a fresh zero, and an empty collection contributes one representative element
from its element/value type. The resolved `Object` is then type-complete (one representative per collection) and each synthesized
`Field.Value` is a non-addressable zero. It is for read-only, type-driven translators (for example a query projection that enumerates nested
paths regardless of runtime contents) — `Strip`, `Clean`, and the in-place fold ignore the flag and fold real instance values only.

```go
// Type-driven translator: nested paths appear even for a zero-value carrier.
eng := behavior.New[bson.M](mongo.NewProjectionTranslator(),
    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
proj, err := eng.Translate(ctx, Entity{})
```

### Pooling

The walk builds its `Object`/`Field` tree on an internal chunked arena. `Strip` and `Clean` always return that arena to a pool: their tree
never escapes the call, so the reuse is invisible to callers. Engines hand the tree to a translator, so for them pooling is opt-in — enable
`WithPooling` on hot paths once the translator honors the retention rule above. `Resolve` always returns a GC-owned tree.

## Traversal semantics

| Field shape                                  | Behavior on field      | Behavior absent                          |
|----------------------------------------------|------------------------|------------------------------------------|
| Scalar; opaque struct (`time.Time`, `Optional`) | Field cleared       | No-op (leaf)                             |
| Nested struct / `*struct`                    | Whole field cleared    | Recursive descent                        |
| Slice/array of structs                       | Field cleared          | Recursive descent into each element      |
| Map with struct values                       | Field cleared          | Recursive descent into each value        |
| Nested collections (`[][]T`, `map[K][]T`, …) | Field cleared          | Recursive descent down to struct elements |

A `behavior` tag placed on a field *inside* an `Optional` is never applied — `Optional` is an opaque leaf and is only cleared when the field
holding it is itself tagged.

Tags on unexported fields are validated (a typo still fails loudly) but otherwise ignored — unexported fields cannot be set and never
participate in a strip. Unlike `encoding/json`, name shadowing between an outer field and a field promoted from an embedded struct is not
resolved: both fields are processed independently.

## Usage

```go
type Bucket struct {
    ID         string                    `behavior:"identifier"`
    TenantID   string                    `behavior:"immutable"`
    CreateTime time.Time                 `behavior:"output_only"`
    Password   string                    `behavior:"input_only"`
    Policy     optional.Optional[Policy]
}

// Reject server-owned fields the client must not supply.
if err := behavior.StripCreate(&bucket); err != nil {
    return err
}

// Report instead of mutate.
if err := behavior.StripCreate(&bucket, behavior.WithStrict()); err != nil {
    var ve *behavior.ViolationError
    if errors.As(err, &ve) {
        // ve.Violations carries each offending Path and Kind.
    }
}
```

## See also

- [`domain/proto/fieldbehavior`](../proto/fieldbehavior/README.md) — the `proto.Message` equivalent driven by `google.api.field_behavior`.
- [`core/types/optional`](../../core/types/optional) — the `Some`/`None` type that round-trips cleanly through a strip.
