# meili

```go
import "github.com/altessa-s/go-atlas/data/orderby/translators/meili"
```

Translates an `orderby.Spec` value into the Meilisearch `sort` argument shape: a `[]string` of `"field:asc"` / `"field:desc"` entries in the order
they appear in the source.

## Usage

```go
parser, _ := orderby.NewParser()
ob, _ := parser.Parse(ctx, "createdAt desc, name")

trans, err := meili.NewTranslator(
    orderby.WithAllowedFields("createdAt", "name"),
)
if err != nil {
    return err
}
sort, _ := trans.Translate(ob)
// sort == []string{"createdAt:desc", "name:asc"}

req := &meilisearch.SearchRequest{Sort: sort}
```

An empty `Spec` translates to a `nil` slice — callers can pass it straight through.
