# normalizer

```go
import "github.com/altessa-s/go-atlas/domain/normalizer"
```

Package `normalizer` provides tag-based string normalization for struct fields. Fields annotated with
`normalize:"..."` tags are processed by `Normalize`. Multiple modifiers can be chained with commas.

## Special tag values

| Tag       | Behavior                                              |
|-----------|-------------------------------------------------------|
| `"-"`     | Skip the field entirely                               |
| `"custom"`| Skip built-in modifiers, only call `CustomNormalizer` |

## CustomNormalizer interface

Struct types can implement `CustomNormalizer` for programmatic normalization. The method is called after
all tag-based modifiers have been applied.

## Built-in modifiers

| Modifier                | Description                                     |
|-------------------------|-------------------------------------------------|
| `trim`                  | Trim leading/trailing whitespace                |
| `lowercase`             | Convert to lowercase                            |
| `uppercase`             | Convert to uppercase                            |
| `nil_on_empty`          | Set `*string` to nil when empty                 |
| `phone`                 | Normalize to E.164 format (param: `region`)     |
| `remove_bad_symbols`    | Strip control chars and deprecated Unicode      |
| `remove_empty_elements` | Remove empty/nil entries from string slices     |

Custom modifiers can be registered via `modifiers.RegisterModifier`.

## Traversal

Nested structs, slices, and arrays are traversed recursively. Struct field metadata is cached for
optimal performance on repeated calls.

## Subpackages

| Package                    | Description                                  |
|----------------------------|----------------------------------------------|
| [modifiers](./modifiers)   | Built-in and custom modifier functions       |
