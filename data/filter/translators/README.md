# translators

Filter expression translators that convert `filter.Node` AST trees into backend-specific query formats using the visitor pattern.

## Subpackages

| Package                    | Description                                            |
|----------------------------|--------------------------------------------------------|
| [clickhouse](./clickhouse) | Translates AST to ClickHouse SQL `WHERE` clauses       |
| [lua](./lua)               | Translates AST to Lua boolean expressions              |
| [mariadb](./mariadb)       | Translates AST to MariaDB / MySQL `WHERE` clauses      |
| [meili](./meili)           | Translates AST to Meilisearch filter expressions       |
| [mongo](./mongo)           | Translates AST to MongoDB `bson.M` queries             |
| [postgres](./postgres)     | Translates AST to PostgreSQL `WHERE` clauses           |
| [redisearch](./redisearch) | Translates AST to RediSearch query syntax              |

## SQL dialects

`clickhouse`, `mariadb` and `postgres` share one AST walk, in the internal package
[`internal/sqlbase`](./internal/sqlbase). Each contributes only a `Dialect` — identifier quoting, bind-marker spelling, string predicates and
inline literal rendering. Their `Translate` returns three values, `(where string, args []any, err error)`; the non-SQL translators return a
single rendered string or a `bson.M`.
