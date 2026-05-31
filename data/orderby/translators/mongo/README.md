# mongo

```go
import "github.com/altessa-s/go-atlas/data/orderby/translators/mongo"
```

Translates an `orderby.Spec` value into a MongoDB `bson.D` sort document. Sort key precedence is preserved because `bson.D` is an ordered
document — Mongo's `sort` option ignores key order on `bson.M`.

## Usage

```go
parser, _ := orderby.NewParser()
ob, _ := parser.Parse(ctx, "create_time desc, slug")

trans, err := mongo.NewTranslator(
    orderby.WithAllowedFields("create_time", "slug"),
)
if err != nil {
    return err
}
sort, _ := trans.Translate(ob)
// sort == bson.D{{"create_time", -1}, {"slug", 1}}

col.Find(ctx, filter, options.Find().SetSort(sort))
```

Ascending keys emit `int32(1)`, descending keys emit `int32(-1)`. An empty `Spec` translates to a `nil` `bson.D` — matching the Meilisearch
translator's "nil for empty" shape so call sites use the same `if sort != nil` check across backends. Passing `nil` to `SetSort` is valid and
equivalent to omitting the sort entirely.
