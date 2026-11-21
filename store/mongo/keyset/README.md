# keyset

```go
import "github.com/altessa-s/go-atlas/store/mongo/keyset"
```

Reads MongoDB pages with a find query and bound continuation tokens. Uses the backend-neutral token contract from
go-aip [pagination/pagetoken](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken).

## Reading a page

```go
codec, err := pagetoken.New(key)
if err != nil {
    return err
}
page, err := keyset.FindPage[User](ctx, collection, codec, keyset.Query{
    Filter: bson.D{{Key: "group", Value: "staff"}},
    Limit:  100,
})
```

Pass `page.Next` in `Query.After` to read the next page with the same filter, sort, precision, and subject.
An empty token marks the last page. A nil codec reads one page. The caller's context controls the query deadline.
Concurrent writes can change which documents appear on later pages.

| Query field          | Contract |
| -------------------- | -------- |
| `Limit`              | Defaults to 100 and is capped at 10,000. |
| `TiebreakPath`       | Unique final sort path. Defaults to `_id` and is appended ascending when absent. |
| `Projection`         | Must retain every sort field with its stored value. Dropping or rewriting one fails with `ErrProjection` before the find. |
| `TimestampPrecision` | Defaults to milliseconds. Positive whole multiples of one millisecond are supported. |

Each sort path must hold a non-null scalar of a consistent BSON type. A missing key or a stored date
misaligned to the requested precision yields `ErrMissingKey`.

## Migrating keyset filters

**Breaking change.** These embedded-value rules also apply to the [entity and list readers](../README.md#read-filters). For keyset, `Query.Filter` remains a `bson.D`; nil matches every document. For embedded documents use
`bson.D`, `bson.M`, `map[string]any`, or pre-built `bson.Raw`. Arbitrary structs and custom BSON marshalers
are rejected. Replace document-shaped values with `bson.D` or encode them to `bson.Raw` in trusted code.
For a trusted scalar struct, register its encoder and call
`bsonfilter.AdmitScalar(reg, reflect.TypeFor[MyScalar]())` from `github.com/altessa-s/go-mongox/bsonfilter`
on that same registry. Atlas date and decimal codecs already work with the default registry.

Atlas clients supply their registry and BSON options. For external clients and database or collection overrides,
pass the effective `Query.Registry` and `Query.BSONOptions`. The prepared bytes bind the token and execute the find.

Maps are sorted by key at every depth. Ordered documents and raw BSON preserve field order, which matters for
MongoDB embedded-document equality. Tokens issued with the same ordered filter continue to resolve. An existing
token whose map encoded in a different order fails with `pagetoken.ErrFilterChanged`; restart pagination with
an empty `After`. The standard token TTL is 24 hours.

## Related packages

- [store/mongo](../README.md)
- [go-aip pagination guide](https://github.com/altessa-s/go-aip/tree/develop/docs/pagination.md)
