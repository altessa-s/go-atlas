# keyset

```go
import "github.com/altessa-s/go-atlas/store/mongo/keyset"
```

Reads MongoDB pages with a find query and bound continuation tokens. Uses the backend-neutral token contract from
go-aip [pagination/pagetoken](https://github.com/altessa-s/go-aip/tree/develop/pagination/pagetoken).

## Contract

`FindPage[T]` reads a MongoDB page through the Collection port and optionally issues a continuation token through TokenCodec. Query carries
filter, sort, projection, size, bindings, registry, and BSON options. Import this package directly; pagination types are not re-exported from
store/mongo.

Sort keys must have comparable, consistent BSON scalar types and survive projection. TiebreakPath defaults to `_id`; it must be unique, non-null, and
included in the projection. The pager stops at an existing identity sort key or appends it ascending. Native BSON identity types, including integer
and
ObjectID, remain typed.

TimestampPrecision zero selects BSON milliseconds; positive multiples of a millisecond are supported. Stored cursor dates must align or FindPage returns
ErrMissingKey. Precision participates in bindings. Filter serialization uses the configured registry and BSON options for both execution and
bindings; use deterministic ordered documents.

FindPage uses the caller context for the entire query and adds no deadline. The listing MongoDB engine owns its WithMaxTime cap. A nil codec returns one
page without a token. Continuation tokens do not provide snapshot isolation.

Sort fields must contain comparable scalars of consistent BSON types and survive the projection. `TiebreakPath` selects the unique tie-breaker; its
default is `_id`. Filter bytes are encoded with the requested registry and BSON options, used both in the query and token binding; use deterministic
ordered documents. A nil codec reads a single page without issuing a token. Pagination is not a snapshot.

Import this package directly; the root does not re-export keyset types or functions.

## Identity path

| Query field                | Contract                                         |
| -------------------------- | ------------------------------------------------ |
| `Query.TiebreakPath` | Unique final BSON sort path; empty selects `_id` |

The pager stops the sort at the configured identity or appends it ascending when absent. The identity must be a non-null scalar,
unique in the result set, and included in the projection. Listing engines may append it with their requested direction before calling the pager.

## Cursor timestamp precision

| Setting                          | Behavior                                                                              |
| -------------------------------- | ------------------------------------------------------------------------------------- |
| `Query.TimestampPrecision` | Zero defaults to BSON milliseconds; positive multiples of a millisecond are supported |

Stored cursor dates must align to this precision. A misaligned date fails with `ErrMissingKey`, so a lossy cursor cannot repeat rows.
Precision is part of token bindings. The unique tie-breaker retains its native BSON scalar type, including integer and ObjectID.

`FindPage` uses the caller context for the entire query and adds no deadline. Direct callers should supply a context deadline when needed.

## Defaults and limits

| Constant                | Value |
| ----------------------- | ----- |
| `DefaultPageSize` | `100` |

## Named values

| Constant            | Meaning                                                |
| ------------------- | ------------------------------------------------------ |
| `MaxPageSize` | MaxPageSize caps the page size [FindPage] reads. |

## API

| Symbol        | Purpose                                                                                                                                                                                                                                                                                                                                                   |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Query` | Query describes one page of a `FindPage` read.                                                                                                                                                                                                                                                                                                      |
| `Page`  | Page is one page of a `FindPage` read.                                                                                                                                                                                                                                                                                                              |
| `TokenCodec`  | TokenCodec issues and resolves bound page tokens.                                                                                                                                                                                                                                                                                                         |
| `FindPage`    | FindPage reads one page of coll with a plain find, positioned by a signed page token rather than an offset. Unlike mongo.ListCursor it works with any "_id" type (ObjectID, string, UUID), needs no created_at field, and streams documents instead of collecting them into one aggregation result, so the 16 MB document limit does not apply to a page. |

| Symbol          | Purpose                                                 |
| --------------- | ------------------------------------------------------- |
| `FindPage[T]`   | Read a page and optionally issue its continuation token |
| `Query`   | Filter, sort, projection, size and token bindings       |
| `Page[T]` | Items and next token                                    |
| `TokenCodec`    | Token issuing and resolution port                       |

## Related packages

- [store/mongo](../README.md)
- [go-aip pagination guide](https://github.com/altessa-s/go-aip/tree/develop/docs/pagination.md)

## MongoDB page tokens

```go
codec, err := pagetoken.New(key /*, options */)
page, err := keyset.FindPage[User](ctx, collection, codec, keyset.Query{Limit: 100})
```
