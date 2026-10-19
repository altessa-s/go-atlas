# mongo

```go
import "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
```

Package `mongo` provides [`behavior.Translator`](../../README.md) implementations that fold behavior-tagged Go structs into MongoDB BSON documents from
a single [`domain/behavior`](../../README.md) reflection walk. It is self-contained: it depends only on the behavior core and the BSON driver, performs
no encryption, and has no dependency on `data/mongo`.

The package exposes **only translators** — drive them through `behavior.New`. Configuration that belongs to the core (the behavior tag name, traversal
depth, and the strip-kind policy) is set on the engine via `behavior.WithTagName` / `behavior.WithMaxDepth` / `behavior.WithKinds`, never duplicated
here. The translators own only the bson tag.

## Translators

| Constructor               | Output                                 | Engine kinds to pass                                                                 |
|---------------------------|----------------------------------------|--------------------------------------------------------------------------------------|
| `NewInsertTranslator`     | `bson.M` insert document               | none — the codec's own encoding of the value                                         |
| `NewUpdateTranslator`     | `bson.M` `{"$set", "$unset"}` document | `behavior.WithKinds(behavior.DefaultUpdateKinds...)`                                 |
| `NewProjectionTranslator` | `bson.M` exclusion projection `{p: 0}` | `behavior.WithKinds(behavior.DefaultResponseKinds...)` + `behavior.WithSchemaWalk()` |
| `NewFieldPathsTranslator` | `FieldPaths{Selectable, Denied}`       | `behavior.WithKinds(behavior.DefaultResponseKinds...)` + `behavior.WithSchemaWalk()` |

The update translator excludes `identifier` fields itself, whatever kinds the engine strips: the identifier selects the record and never lands in `$set`
or `$unset`, under `_id` or any other name, set or nil — including every key an inline struct or inline map tagged `identifier` contributes to the
parent. `DefaultUpdateKinds` keeps the identifier in the `StripUpdate` payload (AIP-203), and the same set drives the update translator. An identifier
inside a collection element belongs to the element and is written with it.

The first three return `behavior.Translator[bson.M]`. `NewProjectionTranslator` and `NewFieldPathsTranslator` are type-driven: the engine **must** be
built with `behavior.WithSchemaWalk()` so nested paths appear even for a zero-value carrier or a nil pointer / empty collection.

## Options

| Option            | Default        | Description                                                                 |
|-------------------|----------------|-----------------------------------------------------------------------------|
| `WithBsonTagName` | `"bson"`       | Struct tag key for the document name plus `omitempty` / `omitonupdate`.     |
| `WithRegistry`    | driver default | BSON registry the driver encodes with; see [Custom codecs](#custom-codecs). |

## Custom codecs

The translators fold a struct field by field unless the codec encodes it as a unit. Pass the registry the driver uses —
`mongo.WithRegistry(bsoncodec.NewRegistry())` with [`data/mongo/bsoncodec`](../../../../data/mongo/bsoncodec) — so that a type it encodes with an
encoder of its own (`decimal.Decimal`, `optional.Optional[T]`) is written as a unit in update documents and projections, and the insert document is
encoded with it. Without it, such a type is folded as an ordinary struct: a `decimal.Decimal` has no exported fields and becomes `{}`.

## Insert documents

An insert applies no behavior filtering, so under the default `bson` tag the insert document **is** the BSON codec's encoding of the value — `inline`,
`omitempty`, `minsize`, marshalers, embedded structs and nil/empty collections exactly as the driver stores them — decoded into a `bson.M`: nested
documents are ordered `bson.D`, arrays `bson.A`, and values carry their BSON Go types (`bson.DateTime` for `time.Time`, `bson.Binary` for `[]byte`,
`int32` for small `int`, a pointer's target instead of the pointer). Keys starting with `$` are rejected at any depth. With `WithBsonTagName` the codec
cannot read the custom tag, so the document is folded field by field under the rules below instead.

## Document rules

The rules below describe the update document, and the insert document under a custom tag.

| Field shape                          | Insert                                            | Update                                                        |
|--------------------------------------|---------------------------------------------------|---------------------------------------------------------------|
| Scalar / opaque struct (`time.Time`) | value via `Interface()`                           | `$set` value                                                  |
| `bool`                               | `bool` value                                      | `$set` value                                                  |
| `[]byte` / `[]uint8`                 | Binary value                                      | `$set` Binary value                                           |
| nil pointer                          | nil value (unless `omitempty`)                    | `$unset`; protected field in its type: `ErrProtectedUnset`    |
| `*struct` (non-nil, recursable)      | nested subdocument                                | nested subdocument (full replace, no nested `$unset`)         |
| `*struct` holding a protected field  | nested subdocument                                | descendant paths `home.city` in `$set` / `$unset`             |
| `[]struct` / `[N]struct` (non-empty) | array of subdocuments                             | array of subdocuments (full replace)                          |
| `map[string]struct` (non-empty)      | subdocument; `$`-prefixed keys rejected           | subdocument (full replace)                                    |
| collection of a stripped field       | written (insert is unfiltered)                    | `ErrProtectedInCollection`                                    |
| empty slice / map / array            | `[]` / `{}`; nil → null (absent with `omitempty`) | `$unset`; protected element field: `ErrProtectedInCollection` |
| `omitonupdate` tag                   | written                                           | skipped; protected, in an element: `ErrProtectedInCollection` |
| stripped field (engine `WithKinds`)  | written (insert is unfiltered)                    | excluded from `$set` and `$unset`                             |
| `identifier` field                   | written (insert is unfiltered)                    | excluded from `$set` and `$unset`, whatever the kinds         |
| `inline` struct / non-nil `*struct`  | fields at the parent level                        | fields at the parent level in `$set` / `$unset`               |
| `inline` nil `*struct`               | nothing                                           | nothing (the codec stores no key to `$unset`)                 |
| `inline` `map[string]T`              | entries at the parent level                       | entries at the parent level in `$set`; empty map: nothing     |

An update never deletes the stored value of a field it leaves out. A nested struct (value or non-nil pointer) that holds a *protected* field — one the
engine stripped, an `identifier`, or one tagged `omitonupdate`, directly or in a nested struct below it — is therefore not replaced whole: its other
fields are written by descendant paths (`$set: {"profile.name": …}`, `$unset: {"profile.nick": …}`), so the stored `profile.uid` survives, and a struct
holding nothing else contributes no key at all. The check follows the BSON layout: `omitonupdate` propagated from an inline struct counts, so does a key
behind an unset inline pointer (the replacement would remove it), and a `bson:"-"` field counts for nothing. A nested struct without protected fields
keeps the full replacement.

A value that removes what is stored in its place — a nil pointer, a nil or empty collection, a nil collection element, at any depth — has no resolved
fields whose strip marks could be checked, so the update translator judges it from its declared type, under the same policy the engine applies to
resolved values: `behavior.New` binds its tag key and strip kinds to the translator (`behavior.PolicyBinder`). When the type holds a protected field —
one with a kind the engine strips, or, outside a collection, an `identifier` or `omitonupdate` one; directly, through an inline struct, or in a struct,
pointer or collection below it — the update fails instead of deleting the stored value: `ErrProtectedUnset` for a subdocument,
`ErrProtectedInCollection` for a collection or for anything inside a replaced element. Send the subdocument instead (it is then written by path, so
`Profile: &Profile{}` clears its other fields and keeps the protected ones), leave the field out of the update (`omitonupdate`), or drop the kind from
the engine's strip set. A nil pointer or empty collection whose type holds nothing protected is unset as before, so replacing a collection and clearing
it follow one policy: under `WithKinds()` both are allowed for elements with `output_only` fields, under `DefaultUpdateKinds` both fail.

A slice, array or map is replaced whole, and no document path addresses a field in each of its elements (map keys are dynamic, and an element path would
merge instead of replace). An `identifier` inside an element is the element's own and is written with it. When the engine strips it (a strip set holding
`Identifier`), it is written only if `identifier` is its sole kind and its value is a unit (a scalar or codec leaf): a stripped field is a leaf to the
engine, so a stripped compound identifier — nested struct, inline struct or inline map — has unchecked children. Any other field the engine stripped
inside the elements — at any depth, in nested collections and in structs inside elements, including an `identifier` also tagged `output_only` or
`immutable` — cannot be preserved, so the update fails with `ErrProtectedInCollection` instead of silently dropping it. So does an `omitonupdate` field
inside an element, which the replacement drops with its stored value, when the engine stripped it or it holds a protected field below it —
`omitonupdate` propagated from an inline struct and an `omitonupdate` inline map included; one holding nothing protected is left out as before. Tag such
a collection `omitonupdate` and update its elements with positional operators, or leave the kind out of the engine's strip set.

Documents follow the layout of the v2 BSON struct codec, computed per struct type: an `inline` struct's fields sit at the parent level and the
shallowest field of a name dominates (an own field, even an omitted or stripped one, shadows an inlined one); two fields of one name at the same depth,
an inline map key naming any field, a second inline map, a non-`string` map key, or `inline` on any other type is an error — decided by the type, not by
which values are set. An inline map nested inside an inline struct is not written, as in the codec. An embedded struct *without* `inline` is a
subdocument named after its type. One deliberate difference: map keys starting with `$` are rejected, inline or not.

A top-level key of `$set` / `$unset`, and every segment of a projection path, is read by MongoDB as a path. A stored name the codec writes literally but
MongoDB would split — one containing a dot (`bson:"a.b"`, or an inline map key such as `"guarded.owner"`) — or one starting with `$` cannot be
addressed: the update translator and the path translators fail with `ErrUnaddressableName` instead of writing to, or exposing, another field. Inserts
store dotted names literally, as the driver does, and reject a name starting with `$`.

`omitempty` is honored on insert only and follows the BSON codec's emptiness rule: a nil pointer or interface, an empty string, collection or array, a
zero scalar, or a `bson.Zeroer` whose `IsZero` reports true is omitted. A struct is never empty unless it is a `Zeroer` (`time.Time` is), and a non-nil
pointer is omitted only when it implements `bson.Zeroer` and `IsZero` reports true. On update `omitempty` has no effect — a nil pointer maps to
`$unset`.

A value the translator writes as is keeps the codec's encoding: an addressable value whose BSON marshaler is declared on the pointer receiver is encoded
by that marshaler, and so is a `minsize` field, both through the codec itself.

The `$`-prefix key guard applies to map keys the translator folds itself. Values written opaquely — interface fields, `[]any` elements, self-marshaling
types — go to the driver as-is; MongoDB treats keys inside such nested values as literal data, not as update operators.

On update, `_id` and every `identifier` field are removed from `$set` and `$unset`, and `$unset` is omitted when empty. No path rooted at the root `_id`
is ever written: a composite `_id` subdocument is not expanded into `_id.<field>` paths, whether the field is an own one or an inline struct contributes
it. Field names come from the bson tag, falling back to the lowercased Go field name when the tag name is empty; only the complete tag `bson:"-"` skips
the field.

## Projection paths

The projection emits a dot-notation path per stripped field. Nested struct and pointer-to-struct fields are descended (`home.secret`). Slice of struct
fields use the field path **without an index** (`aliases.secret`): MongoDB applies the exclusion across every element of the array. Map values sit under
dynamic keys (`labels.<key>.secret`) that no path can address, so a map whose values carry a stripped field is excluded **whole** (`labels`). Because
the engine runs in schema-walk mode, nested paths appear even when the pointer is nil or the collection is empty at run time. Dynamic element types
(`[]any`, `map[string]any`, interface) are leaves and are not descended.

Paths follow the BSON codec's layout of each struct: a `bson:",inline"` struct contributes its dominant fields at the parent level (a field shadowed by
a shallower one of the same name yields no path, and a stripped inline struct denies each of its keys), while an embedded struct *without* `inline` is a
subdocument (`credentials.secret`). An inline map has dynamic keys at the parent level and yields no path; stripped data in one — the map itself or a
field of its values — cannot be excluded, so both translators fail with `ErrStrippedInline`.

## Field paths for data/projection

`NewFieldPathsTranslator` derives a [`data/projection`](../../../../data/projection) policy from the tags. `Denied` lists the stripped paths (the keys
`NewProjectionTranslator` would exclude); `Selectable` lists every other path that neither is, contains, nor sits inside a stripped field — a struct
holding a stripped field is absent, its other fields are listed. Paths are storage (bson) names, without indexes or map keys.

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
