# translators

Filter expression translators that convert `filter.Node` AST trees into backend-specific query formats using the visitor pattern.

## Subpackages

| Package                      | Description                                 |
|------------------------------|---------------------------------------------|
| [lua](./lua)                 | Translates AST to Lua boolean expressions   |
| [mongo](./mongo)             | Translates AST to MongoDB `bson.M` queries  |
| [redisearch](./redisearch)   | Translates AST to RediSearch query syntax   |
