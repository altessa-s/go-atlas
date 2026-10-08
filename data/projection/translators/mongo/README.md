# mongo

```go
import "github.com/altessa-s/go-atlas/data/projection/translators/mongo"
```

Translates a [`projection.Spec`](../../README.md) into a MongoDB projection document (`bson.M`) for an aggregation `$project` stage or a find
projection. The document is a pure field selection: no `$`, `$slice`, `$elemMatch` or expression is ever emitted.

## Output

| Request / policy                                   | Output                                          |
|----------------------------------------------------|-------------------------------------------------|
| Non-empty request                                  | `{path: 1, …}` plus required fields             |
| Empty request, `WithDefaultFields` or allow-list   | `{path: 1, …}` of the defaults plus required    |
| Empty request, no allow-list, denied fields        | `{denied: 0, …}`                                |
| Empty request, no allow-list, nothing denied       | `nil` — no `$project` stage                     |

In inclusion mode `_id` is suppressed (`{_id: 0}`) unless a selected path covers it — keep it with `projection.WithRequiredFields("_id")`.
Paths covered by a selected ancestor are dropped after field mapping, so the document never triggers MongoDB's "Path collision" error.

## Usage

```go
trans, err := mongo.NewTranslator(
    projection.WithUntrustedInput(),
    projection.WithAllowedFields("name", "createTime", "address.*"),
    projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
    projection.WithRequiredFields("_id", "created_at"), // cursor ID and sort key
)
proj, err := trans.Translate(spec)

res, err := datamongo.ListCursor[Doc](ctx, col,
    datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: -1}}),
    datamongo.WithListCursorProjection(proj),
)
```

`data/mongo.ListCursor` rejects a projection that drops its cursor ID field or primary sort field with `ErrProjectionDropsCursorField`, since the
next-page cursor is read from the last returned row.
