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
| `NewInsertTranslator`      | `bson.M` insert document                | none — every present field is written                 |
| `NewUpdateTranslator`      | `bson.M` `{"$set", "$unset"}` document  | `behavior.WithKinds(behavior.DefaultUpdateKinds...)`  |
| `NewProjectionTranslator`  | `bson.M` exclusion projection `{p: 0}`  | `behavior.WithKinds(behavior.DefaultResponseKinds...)` + `behavior.WithSchemaWalk()` |

All three return `behavior.Translator[bson.M]`. `NewProjectionTranslator` is type-driven: the engine **must** be built with
`behavior.WithSchemaWalk()` so nested paths appear even for a zero-value carrier or a nil pointer / empty collection.

## Options

| Option            | Default  | Description                                                            |
|-------------------|----------|-----------------------------------------------------------------------|
| `WithBsonTagName` | `"bson"` | Struct tag key for the document name plus `omitempty` / `omitonupdate`. |

## Document rules

| Field shape                          | Insert                                  | Update                                                      |
|--------------------------------------|-----------------------------------------|------------------------------------------------------------|
| Scalar / opaque struct (`time.Time`) | value via `Interface()`                 | `$set` value                                               |
| `bool`                               | `bool` value                            | `$set` value                                               |
| `[]byte` / `[]uint8`                 | Binary value                            | `$set` Binary value                                        |
| nil pointer                          | nil value (unless `omitempty`)          | `$unset`                                                   |
| `*struct` (non-nil, recursable)      | nested subdocument                      | nested subdocument (full replace, no nested `$unset`)      |
| `[]struct` / `[N]struct` (non-empty) | array of subdocuments                   | array of subdocuments (full replace)                       |
| `map[string]struct` (non-empty)      | subdocument; `$`-prefixed keys rejected | subdocument (full replace)                                 |
| empty slice / map / array            | absent                                  | `$unset`                                                   |
| `omitonupdate` tag                   | written                                 | skipped                                                    |
| stripped field (engine `WithKinds`)  | written (insert is unfiltered)          | excluded from `$set` and `$unset`                          |

Strip exclusion applies recursively: nested subdocuments — struct pointers, slice elements, and map values alike — are folded from the
engine's resolved objects, so stripped fields are dropped at every nesting level.

`omitempty` is honored on insert only and matches the BSON driver: any zero-value field (zero scalar, nil pointer, zero struct such as
`time.Time{}`) is omitted. A non-nil pointer is never omitted, even when it points at a zero value. On update `omitempty` has no effect —
a nil pointer maps to `$unset`.

The `$`-prefix key guard applies to map keys the translator folds itself. Values written opaquely — interface fields, `[]any` elements,
self-marshaling types — go to the driver as-is; MongoDB treats keys inside such nested values as literal data, not as update operators.

On update, `_id` is removed from `$set` and `$unset` is omitted when empty. Field names come from the bson tag, falling back to the
lowercased Go field name; a `-` or empty name is skipped.

## Projection paths

The projection emits a dot-notation path per stripped field. Nested struct and pointer-to-struct fields are descended (`home.secret`).
Slice and map of struct fields use the field path **without an index** (`aliases.secret`, `labels.secret`): MongoDB applies the exclusion
across every embedded document reachable by that path. Because the engine runs in schema-walk mode, nested paths appear even when the
pointer is nil or the collection is empty at run time. Dynamic element types (`[]any`, `map[string]any`, interface) are leaves and are not
descended.

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
