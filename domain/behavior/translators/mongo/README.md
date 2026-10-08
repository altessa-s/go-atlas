# mongo

```go
import "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
```

Package `mongo` provides [`behavior.Translator`](../../README.md) implementations that fold behavior-tagged Go structs into MongoDB BSON
documents from a single [`domain/behavior`](../../README.md) reflection walk. It is self-contained: it depends only on the behavior core
and the BSON driver, performs no encryption, and has no dependency on `data/mongo`.

The package exposes **only translators** — drive them through `behavior.New`. Configuration that belongs to the core (the behavior tag
name, traversal depth, and the strip-kind policy) is set on the engine via `behavior.WithTagName` / `behavior.WithMaxDepth` /
`behavior.WithKinds`, never duplicated here. The translators own only the bson tag.

## Translators

| Constructor                | Output                                  | Engine kinds to pass                                  |
|----------------------------|-----------------------------------------|-------------------------------------------------------|
| `NewInsertTranslator`      | `bson.M` insert document                | none — the codec's own encoding of the value          |
| `NewUpdateTranslator`      | `bson.M` `{"$set", "$unset"}` document  | `behavior.WithKinds(behavior.DefaultUpdateKinds...)`  |
| `NewProjectionTranslator`  | `bson.M` exclusion projection `{p: 0}`  | `behavior.WithKinds(behavior.DefaultResponseKinds...)` + `behavior.WithSchemaWalk()` |
| `NewFieldPathsTranslator`  | `FieldPaths{Selectable, Denied}`        | `behavior.WithKinds(behavior.DefaultResponseKinds...)` + `behavior.WithSchemaWalk()` |

The first three return `behavior.Translator[bson.M]`. `NewProjectionTranslator` and `NewFieldPathsTranslator` are type-driven: the engine
**must** be built with `behavior.WithSchemaWalk()` so nested paths appear even for a zero-value carrier or a nil pointer / empty collection.

## Options

| Option            | Default  | Description                                                            |
|-------------------|----------|-----------------------------------------------------------------------|
| `WithBsonTagName` | `"bson"` | Struct tag key for the document name plus `omitempty` / `omitonupdate`. |

## Insert documents

An insert applies no behavior filtering, so under the default `bson` tag the insert document **is** the BSON codec's encoding of the value —
`inline`, `omitempty`, `minsize`, marshalers, embedded structs and nil/empty collections exactly as the driver stores them — decoded into a
`bson.M`: nested documents are ordered `bson.D`, arrays `bson.A`, and values carry their BSON Go types (`bson.DateTime` for `time.Time`, `bson.Binary`
for `[]byte`, `int32` for small `int`, a pointer's target instead of the pointer). Keys starting with `$` are rejected at any depth. With
`WithBsonTagName` the codec cannot read the custom tag, so the document is folded field by field under the rules below instead.

## Document rules

The rules below describe the update document, and the insert document under a custom tag.

| Field shape                          | Insert                                  | Update                                                      |
|--------------------------------------|-----------------------------------------|------------------------------------------------------------|
| Scalar / opaque struct (`time.Time`) | value via `Interface()`                 | `$set` value                                               |
| `bool`                               | `bool` value                            | `$set` value                                               |
| `[]byte` / `[]uint8`                 | Binary value                            | `$set` Binary value                                        |
| nil pointer                          | nil value (unless `omitempty`)          | `$unset`                                                   |
| `*struct` (non-nil, recursable)      | nested subdocument                      | nested subdocument (full replace, no nested `$unset`)      |
| `[]struct` / `[N]struct` (non-empty) | array of subdocuments                   | array of subdocuments (full replace)                       |
| `map[string]struct` (non-empty)      | subdocument; `$`-prefixed keys rejected | subdocument (full replace)                                 |
| empty slice / map / array            | `[]` / `{}`; nil → null (absent with `omitempty`) | `$unset`                                         |
| `omitonupdate` tag                   | written                                 | skipped                                                    |
| stripped field (engine `WithKinds`)  | written (insert is unfiltered)          | excluded from `$set` and `$unset`                          |
| `inline` struct / non-nil `*struct`  | fields at the parent level              | fields at the parent level in `$set` / `$unset`            |
| `inline` nil `*struct`               | nothing                                 | nothing (the codec stores no key to `$unset`)              |
| `inline` `map[string]T`              | entries at the parent level             | entries at the parent level in `$set`; empty map: nothing  |

Strip exclusion applies recursively: nested subdocuments — struct pointers, slice elements, and map values alike — are folded from the
engine's resolved objects, so stripped fields are dropped at every nesting level.

Documents follow the layout of the v2 BSON struct codec, computed per struct type: an `inline` struct's fields sit at the parent level and the
shallowest field of a name dominates (an own field, even an omitted or stripped one, shadows an inlined one); two fields of one name at the same
depth, an inline map key naming any field, a second inline map, a non-`string` map key, or `inline` on any other type is an error — decided by the
type, not by which values are set. An inline map nested inside an inline struct is not written, as in the codec. An embedded struct *without*
`inline` is a subdocument named after its type. One deliberate difference: map keys starting with `$` are rejected, inline or not.

A top-level key of `$set` / `$unset`, and every segment of a projection path, is read by MongoDB as a path. A stored name the codec writes
literally but MongoDB would split — one containing a dot (`bson:"a.b"`, or an inline map key such as `"guarded.owner"`) — or one starting with
`$` cannot be addressed: the update translator and the path translators fail with `ErrUnaddressableName` instead of writing to, or exposing,
another field. Inserts store such names literally, as the driver does.

`omitempty` is honored on insert only and follows the BSON codec's emptiness rule: a nil pointer or interface, an empty string, collection
or array, a zero scalar, or a `bson.Zeroer` whose `IsZero` reports true is omitted. A struct is never empty unless it is a `Zeroer`
(`time.Time` is), and a non-nil pointer is never omitted, even when it points at a zero value. On update `omitempty` has no effect — a nil
pointer maps to `$unset`.

A value the translator writes as is keeps the codec's encoding: an addressable value whose BSON marshaler is declared on the pointer receiver
is encoded by that marshaler, and so is a `minsize` field, both through the codec itself.

The `$`-prefix key guard applies to map keys the translator folds itself. Values written opaquely — interface fields, `[]any` elements,
self-marshaling types — go to the driver as-is; MongoDB treats keys inside such nested values as literal data, not as update operators.

On update, `_id` is removed from `$set` and `$unset` is omitted when empty. Field names come from the bson tag, falling back to the
lowercased Go field name; a `-` or empty name is skipped.

## Projection paths

The projection emits a dot-notation path per stripped field. Nested struct and pointer-to-struct fields are descended (`home.secret`).
Slice of struct fields use the field path **without an index** (`aliases.secret`): MongoDB applies the exclusion across every element of the
array. Map values sit under dynamic keys (`labels.<key>.secret`) that no path can address, so a map whose values carry a stripped field is
excluded **whole** (`labels`). Because the engine runs in schema-walk mode, nested paths appear even when the
pointer is nil or the collection is empty at run time. Dynamic element types (`[]any`, `map[string]any`, interface) are leaves and are not
descended.

Paths follow the BSON codec's layout of each struct: a `bson:",inline"` struct contributes its dominant fields at the parent level (a field
shadowed by a shallower one of the same name yields no path, and a stripped inline struct denies each of its keys), while an embedded struct
*without* `inline` is a subdocument (`credentials.secret`). An inline map has dynamic keys at the parent level and yields no path; stripped
data in one — the map itself or a field of its values — cannot be excluded, so both translators fail with `ErrStrippedInline`.

## Field paths for data/projection

`NewFieldPathsTranslator` derives a [`data/projection`](../../../../data/projection) policy from the tags. `Denied` lists the stripped paths (the
keys `NewProjectionTranslator` would exclude); `Selectable` lists every other path that neither is, contains, nor sits inside a stripped field — a
struct holding a stripped field is absent, its other fields are listed. Paths are storage (bson) names, without indexes or map keys.

```go
fpEng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
fp, err := fpEng.Translate(ctx, User{})
// fp.Selectable == [_id create_time name …], fp.Denied == [password]

trans, err := projmongo.NewTranslator(
    projection.WithUntrustedInput(),
    projection.WithAllowedFields(fp.Selectable...),   // API names equal bson names here; map them otherwise
    projection.WithDeniedStorageFields(fp.Denied...),
)
```

## Usage

```go
type User struct {
    ID         string    `bson:"_id"         behavior:"identifier"`
    Name       string    `bson:"name"`
    Password   string    `bson:"password"    behavior:"input_only"`
    CreateTime time.Time `bson:"create_time" behavior:"output_only"`
}

ins := behavior.New[bson.M](mongo.NewInsertTranslator())
doc, err := ins.Translate(ctx, &user) // all fields

upd := behavior.New[bson.M](mongo.NewUpdateTranslator(),
    behavior.WithKinds(behavior.DefaultUpdateKinds...))
setUnset, err := upd.Translate(ctx, &user) // no _id, no create_time

proj := behavior.New[bson.M](mongo.NewProjectionTranslator(),
    behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
p, err := proj.Translate(ctx, User{}) // {"password": 0}
```

## See also

- [`domain/behavior`](../../README.md) — the reflection core, `WithSchemaWalk`, and `Strip` / `Clean` default outputs.
