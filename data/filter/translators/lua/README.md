# lua

```go
import "github.com/altessa-s/go-atlas/data/filter/translators/lua"
```

Package `lua` translates filter AST nodes into Lua boolean expressions suitable for Redis `EVAL` scripts, so filters run server-side on plain Redis
keys.

## Usage

```go
translator, err := lua.NewTranslator("d", filter.WithAllowedFields("active", "name"))
if err != nil { return err }
expression, err := translator.Translate(node)
// Bind the record to local d in the Lua script before evaluating expression.
```

## Behavior

| Operation                 | Rendering                                                    |
|---------------------------|--------------------------------------------------------------|
| Field reference           | Table access, including bare boolean and right-hand operands  |
| `in`                      | Equality disjunction; an empty list is `false`                |
| String predicates         | Lua string functions with literal arguments                  |
| `size()` in a comparison  | UTF-8 code-point count for strings; element count for arrays  |
| `matches()`               | `filter.ErrUnsupportedOperation`                             |

The output supports Lua 5.1, including Redis `EVAL`; Unicode size needs no `utf8` library. NUL escapes use three digits so following digits retain their
value. Field mappings are escaped as table keys. The table variable name is trusted application configuration.

Limits and field policies are shared with [filter](../../README.md). Construct a translator per concurrent operation; instances keep visitor depth
state.
