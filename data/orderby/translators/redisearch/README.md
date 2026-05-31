# redisearch

```go
import "github.com/altessa-s/go-atlas/data/orderby/translators/redisearch"
```

Translates an `orderby.Spec` value into a single-field RediSearch sort instruction. RediSearch's `FT.SEARCH ... SORTBY` accepts exactly one field;
multi-key inputs return `orderby.ErrTooManySortKeys`.

## Usage

```go
parser, _ := orderby.NewParser()
ob, _ := parser.Parse(ctx, "createdAt desc")

trans, err := redisearch.NewTranslator(
    orderby.WithAllowedFields("createdAt"),
)
if err != nil {
    return err
}
sortBy, _ := trans.Translate(ob)
// sortBy == redisearch.SortBy{Field: "createdAt", Descending: true}

args := []any{"FT.SEARCH", "idx", "*"}
if sortBy.Field != "" {
    direction := "ASC"
    if sortBy.Descending {
        direction = "DESC"
    }
    args = append(args, "SORTBY", sortBy.Field, direction)
}
```

`SortBy.Field` is empty for an empty `Spec`; callers should skip the `SORTBY` clause in that case.
